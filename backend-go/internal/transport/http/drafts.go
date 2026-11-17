package transporthttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/aiclient"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

const (
	maxPDF     = 20 << 20
	maxPDFBody = maxPDF + 1<<20 // 20 MiB PDF ve multipart'ın ek baytları
)

// Extractor, bir sözleşme PDF'ini taslağa çevirir; *aiclient.Client bunu uygular.
type Extractor interface {
	Extract(ctx context.Context, pdf []byte) (aiclient.Result, error)
}

func toDraftResp(d repository.Draft) draftResp {
	return draftResp{ID: d.ID, Status: d.Status, Draft: d.Draft, Warnings: d.Warnings, Meta: d.Meta,
		ErrorCode: d.ErrorCode, ContractID: d.ContractID}
}

func orJSON(v json.RawMessage, fallback string) json.RawMessage {
	if len(v) == 0 || string(v) == "null" {
		return json.RawMessage(fallback)
	}
	return v
}

// aiEnabled, çıkarım kapalıysa 503 cevabını kendisi verir.
func (s *server) aiEnabled(w http.ResponseWriter) bool {
	if s.ai == nil {
		writeErr(w, 503, "AI_DISABLED", "contract extraction is disabled")
		return false
	}
	return true
}

// extendWrite, yavaş bir çıkarımın sunucunun WriteTimeout süresini aşabilmesini sağlar; httptest kaydedicileri bunu
// desteklemiyor.
func (s *server) extendWrite(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(s.extractTimeout + 30*time.Second))
}

// readPDF, boyutu sınırlı multipart gövdeden "file" parçasını okur; 413/400 cevabını kendisi verir ve ok=false döner.
func readPDF(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPDFBody)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be multipart/form-data with a file field")
		return nil, false
	}
	for {
		p, err := mr.NextPart()
		if err == nil && p.FormName() != "file" {
			continue
		}
		var b []byte
		if err == nil {
			b, err = io.ReadAll(p)
		}
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			writeErr(w, 413, "PAYLOAD_TOO_LARGE", "body exceeds 21 MiB")
		case err != nil:
			writeErr(w, 400, "BAD_REQUEST", "body must be multipart/form-data with a file field")
		case len(b) > maxPDF:
			writeErr(w, 413, "PAYLOAD_TOO_LARGE", "PDF exceeds 20 MiB")
		case !bytes.HasPrefix(b, []byte("%PDF-")):
			writeErr(w, 400, "NOT_PDF", "file is not a PDF")
		default:
			return b, true
		}
		return nil, false
	}
}

// extractInto çıkarımı çalıştırır ve sonucunu kaydeder; ikisi de istemcinin bağlantıyı kesmesinden etkilenmez (tasarım
// §6.4).
func (s *server) extractInto(ctx context.Context, tid, id uuid.UUID, pdf []byte) (repository.Draft, error) {
	wctx := context.WithoutCancel(ctx)
	ectx, cancel := context.WithTimeout(wctx, s.extractTimeout)
	res, xerr := s.ai.Extract(ectx, pdf)
	cancel()
	var d repository.Draft
	err := s.store.InTenant(wctx, tid, func(tx *repository.Tx) error {
		var aerr *aiclient.Error
		var err error
		switch {
		case xerr == nil:
			err = tx.SetDraftResult(wctx, id, res.Draft, orJSON(res.Warnings, "[]"), orJSON(res.Meta, "{}"))
		case errors.As(xerr, &aerr):
			err = tx.SetDraftFailed(wctx, id, aerr.Code)
		default:
			err = tx.SetDraftFailed(wctx, id, "AI_UNAVAILABLE")
		}
		if err != nil {
			return err
		}
		d, err = tx.GetDraft(wctx, id)
		return err
	})
	return d, err
}

func (s *server) postDraft(w http.ResponseWriter, r *http.Request) {
	if !s.aiEnabled(w) {
		return
	}
	s.extendWrite(w)
	// yavaş yüklenen 21 MiB'lık bir dosya sunucunun ReadTimeout'una takılmamalı; httptest bunu da yok sayıyor
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(s.extractTimeout))
	ctx, tid := r.Context(), auth.TenantID(r.Context())
	pdf, ok := readPDF(w, r)
	if !ok {
		return
	}
	id := uuid.New()
	// yalnızca tenant UUID'sinden, sabit bir türden ve sunucunun ürettiği bir UUID'den kurulur; istemciden gelen metin
	// asla kullanılmaz
	path := filepath.Join(s.storageDir, tid.String(), "contract", id.String()+".pdf")
	if err := writeRaw(path, pdf); err != nil {
		fail(w, err)
		return
	}
	sum := sha256.Sum256(pdf)
	err := s.store.InTenant(ctx, tid, func(tx *repository.Tx) error {
		return tx.InsertDraft(ctx, id, path, hex.EncodeToString(sum[:]))
	})
	if err != nil {
		os.Remove(path)
		fail(w, err)
		return
	}
	d, err := s.extractInto(ctx, tid, id, pdf)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, toDraftResp(d))
}

func (s *server) listDrafts(w http.ResponseWriter, r *http.Request) {
	var out []repository.DraftSummary
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		out, err = tx.ListDrafts(r.Context())
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *server) getDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	var d repository.Draft
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		d, err = tx.GetDraft(r.Context(), id)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, toDraftResp(d))
}

func (s *server) reextractDraft(w http.ResponseWriter, r *http.Request) {
	if !s.aiEnabled(w) {
		return
	}
	s.extendWrite(w)
	ctx, tid := r.Context(), auth.TenantID(r.Context())
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	var path string
	err := s.store.InTenant(ctx, tid, func(tx *repository.Tx) error {
		d, err := tx.LockDraft(ctx, id)
		if err != nil {
			return err
		}
		if d.Status == repository.DraftApproved {
			return repository.ErrDraftApproved
		}
		path = d.PDFPath
		return tx.SetDraftExtracting(ctx, id)
	})
	if err != nil {
		fail(w, err)
		return
	}
	pdf, err := os.ReadFile(path)
	if err != nil {
		fail(w, err)
		return
	}
	d, err := s.extractInto(ctx, tid, id, pdf)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, toDraftResp(d))
}

// getContractPDF, onaylı bir sözleşmenin okunduğu PDF'i sunar; böylece bir bulgu kaynağındaki sayfayı gösterebilir.
func (s *server) getContractPDF(w http.ResponseWriter, r *http.Request) {
	no := r.URL.Query().Get("contract_no")
	if no == "" {
		writeErr(w, 400, "BAD_REQUEST", "contract_no is required")
		return
	}
	var path string
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		path, err = tx.ContractFilePath(r.Context(), no)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		fail(w, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Disposition", `attachment; filename="sozlesme.pdf"`) // tarayıcıdaki görüntüleyici çeker, bizim adresimizde asla sayfa olarak açılmaz
	h.Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "", st.ModTime(), f)
}
