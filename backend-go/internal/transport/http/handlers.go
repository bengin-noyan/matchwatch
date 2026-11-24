package transporthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/ubl"
)

const (
	maxXMLBytes  = 10 << 20
	maxJSONBytes = 1 << 20
	maxNoteLen   = 2000
	maxLimit     = 200
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code})
}

// fail, repository hatalarını HTTP cevaplarına çevirir; tanımadığı bir hatayı loglar ve genel bir 500 döner.
func fail(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, repository.ErrCustomerMismatch):
		writeErr(w, 422, "CUSTOMER_MISMATCH", "document customer VKN does not match the tenant")
	case errors.Is(err, repository.ErrDuplicateETTN):
		writeErr(w, 409, "DUPLICATE_ETTN", "a document with this ETTN already exists")
	case errors.Is(err, repository.ErrDuplicateContract):
		writeErr(w, 409, "DUPLICATE_CONTRACT", "a contract with this number already exists")
	case errors.Is(err, repository.ErrUnknownDespatch):
		writeErr(w, 404, "DESPATCH_NOT_FOUND", "referenced despatch not found")
	case errors.Is(err, repository.ErrDraftNotReady):
		writeErr(w, 409, "DRAFT_NOT_READY", "draft is not ready for approval")
	case errors.Is(err, repository.ErrDraftApproved):
		writeErr(w, 409, "DRAFT_APPROVED", "draft is already approved")
	case errors.Is(err, repository.ErrNotFound):
		writeErr(w, 404, "NOT_FOUND", "not found")
	case errors.As(err, &pgErr) && pgErr.Code == "23514": // CHECK ihlali, ör. rejected_qty > delivered_qty
		writeErr(w, 422, "INVALID_DOCUMENT", "document violates a data constraint")
	default:
		log.Printf("internal error: %v", err)
		writeErr(w, 500, "INTERNAL", "internal error")
	}
}

// readXML, boyutu sınırlı gövdeyi okur; 413/400 cevabını kendisi verir ve ok=false döner.
func readXML(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxXMLBytes))
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		writeErr(w, 413, "PAYLOAD_TOO_LARGE", "body exceeds 10 MiB")
	case err != nil:
		writeErr(w, 400, "BAD_REQUEST", "could not read body")
	default:
		return b, true
	}
	return nil, false
}

func invalidDoc(w http.ResponseWriter, err error) {
	writeErr(w, 422, "INVALID_DOCUMENT", err.Error())
}

func pathID(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	return id, err == nil
}

// rawPath yalnızca tenant UUID'sinden, sabit bir türden ve ETTN'nin standart halinden kurulur; istemciden gelen metin
// asla kullanılmaz. Tür bilgisi, aynı ETTN'yi taşıyan bir fatura ile irsaliyeyi ayrı tutar (ETTN yalnızca kendi
// tablosunda benzersiz).
func (s *server) rawPath(tid uuid.UUID, kind, ettn string) string {
	return filepath.Join(s.storageDir, tid.String(), kind, ettn+".xml")
}

func writeRaw(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}

// ingest, fn'i tenant transaction'ında çalıştırır. fn, write()'ı ancak veritabanı kaydı kabul ettikten sonra çağırır
// (böylece mükerrer ya da hatalı belgeler diske hiç yazılmaz). Commit yine de başarısız olursa dosya geri silinir.
func (s *server) ingest(ctx context.Context, tid uuid.UUID, path string, raw []byte, fn func(tx *repository.Tx, write func() error) error) error {
	wrote := false
	err := s.store.InTenant(ctx, tid, func(tx *repository.Tx) error {
		return fn(tx, func() error {
			wrote = true
			return writeRaw(path, raw)
		})
	})
	if err != nil && wrote {
		os.Remove(path)
	}
	return err
}

func checkCustomer(ctx context.Context, tx *repository.Tx, customerVKN string) error {
	vkn, err := tx.TenantVKN(ctx)
	if err != nil {
		return err
	}
	if vkn != customerVKN {
		return repository.ErrCustomerMismatch
	}
	return nil
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		log.Printf("healthz: %v", err)
		writeErr(w, 503, "UNAVAILABLE", "database unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *server) postContract(w http.ResponseWriter, r *http.Request) {
	tid := auth.TenantID(r.Context())
	doc, err := repository.ParseContractJSON(http.MaxBytesReader(w, r.Body, maxJSONBytes))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeErr(w, 413, "PAYLOAD_TOO_LARGE", "body exceeds 1 MiB")
		return
	}
	if err != nil {
		writeErr(w, 422, "INVALID_CONTRACT", err.Error())
		return
	}
	name := doc.VendorName
	if name == "" {
		name = doc.VendorVKN
	}
	var id uuid.UUID
	err = s.store.InTenant(r.Context(), tid, func(tx *repository.Tx) error {
		if doc.DraftID != nil {
			d, err := tx.LockDraft(r.Context(), *doc.DraftID)
			switch {
			case err != nil:
				return err
			case d.Status == repository.DraftApproved:
				return repository.ErrDraftApproved
			case d.Status != repository.DraftReady:
				return repository.ErrDraftNotReady
			}
			doc.FilePath = d.PDFPath
		}
		var err error
		if doc.VendorID, err = tx.UpsertVendor(r.Context(), doc.VendorVKN, name); err != nil {
			return err
		}
		if id, err = tx.InsertContract(r.Context(), doc.ContractInput); err != nil {
			return err
		}
		// faturalar çok olabilir; istek içinde denetlemiyoruz, RetryPending toparlar
		ids, err := tx.VendorInvoicesBetween(r.Context(), doc.VendorID, doc.ValidFrom, doc.ValidUntil)
		if err != nil {
			return err
		}
		if err := tx.MarkReaudit(r.Context(), ids); err != nil || doc.DraftID == nil {
			return err
		}
		return tx.ApproveDraft(r.Context(), *doc.DraftID, id)
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, idResp{ID: id})
}

// reaudit, yeni yüklenen bir irsaliyeye atıf yapan her faturayı yeniden denetler.
func (s *server) reaudit(ctx context.Context, tid uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if ids == nil {
		ids = []uuid.UUID{}
	}
	for _, id := range ids {
		if _, err := s.auditor.AuditInvoice(ctx, tid, id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (s *server) postDespatch(w http.ResponseWriter, r *http.Request) {
	ctx, tid := r.Context(), auth.TenantID(r.Context())
	body, ok := readXML(w, r)
	if !ok {
		return
	}
	d, err := ubl.ParseDespatch(bytes.NewReader(body))
	if err != nil {
		invalidDoc(w, err)
		return
	}
	var id uuid.UUID
	var refs []uuid.UUID
	err = s.ingest(ctx, tid, s.rawPath(tid, "despatch", d.ETTN), body, func(tx *repository.Tx, write func() error) error {
		if err := checkCustomer(ctx, tx, d.Customer.VKN); err != nil {
			return err
		}
		v, err := tx.UpsertVendor(ctx, d.Supplier.VKN, d.Supplier.Title)
		if err != nil {
			return err
		}
		if id, err = tx.InsertDespatch(ctx, v, d, s.rawPath(tid, "despatch", d.ETTN)); err != nil {
			return err
		}
		if refs, err = tx.InvoicesReferencingDespatch(ctx, v, d.Number); err != nil {
			return err
		}
		if err = tx.MarkReaudit(ctx, refs); err != nil {
			return err
		}
		return write()
	})
	if err != nil {
		fail(w, err)
		return
	}
	if refs, err = s.reaudit(ctx, tid, refs); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, ingestResp{ID: id, Reaudited: refs})
}

func (s *server) postReceipt(w http.ResponseWriter, r *http.Request) {
	ctx, tid := r.Context(), auth.TenantID(r.Context())
	body, ok := readXML(w, r)
	if !ok {
		return
	}
	rc, err := ubl.ParseReceipt(bytes.NewReader(body))
	if err != nil {
		invalidDoc(w, err)
		return
	}
	var no string
	var refs []uuid.UUID
	err = s.store.InTenant(ctx, tid, func(tx *repository.Tx) error {
		if err := checkCustomer(ctx, tx, rc.Customer.VKN); err != nil {
			return err
		}
		v, err := tx.UpsertVendor(ctx, rc.Supplier.VKN, rc.Supplier.Title) // irsaliye bilinmiyorsa geri alınır
		if err != nil {
			return err
		}
		if refs, err = tx.InvoicesReferencingDespatch(ctx, v, rc.DespatchNumber); err != nil {
			return err
		}
		// ApplyReceipt'ten önce: denetim de önce faturayı, sonra irsaliye satırlarını kilitler (deadlock olmasın)
		if err = tx.MarkReaudit(ctx, refs); err != nil {
			return err
		}
		no, err = tx.ApplyReceipt(ctx, v, rc)
		return err
	})
	if err != nil {
		fail(w, err)
		return
	}
	if refs, err = s.reaudit(ctx, tid, refs); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, receiptResp{DespatchNo: no, Reaudited: refs})
}

func (s *server) postInvoice(w http.ResponseWriter, r *http.Request) {
	ctx, tid := r.Context(), auth.TenantID(r.Context())
	body, ok := readXML(w, r)
	if !ok {
		return
	}
	inv, err := ubl.ParseInvoice(bytes.NewReader(body))
	if err != nil {
		invalidDoc(w, err)
		return
	}
	var id uuid.UUID
	err = s.ingest(ctx, tid, s.rawPath(tid, "invoice", inv.ETTN), body, func(tx *repository.Tx, write func() error) error {
		if err := checkCustomer(ctx, tx, inv.Customer.VKN); err != nil {
			return err
		}
		v, err := tx.UpsertVendor(ctx, inv.Supplier.VKN, inv.Supplier.Title)
		if err != nil {
			return err
		}
		if id, err = tx.InsertInvoice(ctx, v, inv, s.rawPath(tid, "invoice", inv.ETTN)); err != nil {
			return err
		}
		return write()
	})
	if err != nil {
		fail(w, err)
		return
	}
	st, err := s.auditor.AuditInvoice(ctx, tid, id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, invoiceResp{ID: id, AuditStatus: string(st)})
}

func (s *server) listInvoices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repository.InvoiceFilter{Status: q.Get("status"), Limit: 50}
	bad := func(msg string) { writeErr(w, 400, "BAD_REQUEST", msg) }
	switch f.Status {
	case "", "GREEN", "YELLOW", "RED", "PENDING":
	default:
		bad("status must be GREEN, YELLOW, RED or PENDING")
		return
	}
	if v := q.Get("vendor_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			bad("vendor_id must be a UUID")
			return
		}
		f.VendorID = &id
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			bad("limit must be an integer 1..200")
			return
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			bad("offset must be an integer >= 0")
			return
		}
		f.Offset = n
	}
	if f.Query = strings.TrimSpace(q.Get("q")); len(f.Query) > 100 {
		bad("q must be at most 100 characters")
		return
	}
	for _, d := range []struct {
		name string
		dst  **string
	}{{"from", &f.From}, {"to", &f.To}} {
		if v := q.Get(d.name); v != "" {
			if _, err := time.Parse(time.DateOnly, v); err != nil {
				bad(d.name + " must be a date YYYY-MM-DD")
				return
			}
			*d.dst = &v
		}
	}
	switch q.Get("open") {
	case "", "false":
	case "true":
		f.OpenOnly = true
	default:
		bad("open must be true or false")
		return
	}
	if f.Sort = q.Get("sort"); f.Sort != "" && !repository.ValidInvoiceSort(f.Sort) {
		bad("sort must be issue_date, invoice_no, vendor, payable, status or open_findings")
		return
	}
	switch q.Get("dir") {
	case "", "asc":
	case "desc":
		f.Desc = true
	default:
		bad("dir must be asc or desc")
		return
	}
	var items []repository.InvoiceRow
	var total int
	var counts repository.InvoiceCounts
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		items, total, counts, err = tx.ListInvoices(r.Context(), f)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, invoiceListResp{Items: items, Total: total, Counts: counts})
}

func (s *server) getInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	var d repository.InvoiceDetailRow
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		d, err = tx.InvoiceDetail(r.Context(), id)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *server) auditInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	st, err := s.auditor.AuditInvoice(r.Context(), auth.TenantID(r.Context()), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"audit_status": string(st)})
}

// decisionErr bir kararı doğrular; "" geçerli demek.
func decisionErr(status, note string) string {
	switch domain.Resolution(status) {
	case domain.ResolutionOpen, domain.ResolutionDisputed, domain.ResolutionApprovedException:
	default:
		return "resolution_status must be OPEN, DISPUTED or APPROVED_EXCEPTION"
	}
	if len(note) > maxNoteLen {
		return "note exceeds 2000 bytes"
	}
	return ""
}

// decider, isteği yapan kullanıcıdır: web girişleri kullanıcı id'sini taşır, dev token'lar taşımaz.
func decider(r *http.Request) *uuid.UUID {
	if u, err := uuid.Parse(auth.Subject(r.Context())); err == nil {
		return &u
	}
	return nil
}

func (s *server) patchFinding(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	var req patchReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be a JSON object")
		return
	}
	if msg := decisionErr(req.ResolutionStatus, req.Note); msg != "" {
		writeErr(w, 422, "INVALID_RESOLUTION", msg)
		return
	}
	_, err := s.auditor.Resolve(r.Context(), auth.TenantID(r.Context()), []uuid.UUID{id}, domain.Resolution(req.ResolutionStatus), req.Note, decider(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, patchResp{ID: id, ResolutionStatus: req.ResolutionStatus})
}

// postDecisions, tek bir kararı aynı anda en fazla maxLimit bulguya uygular; bilinmeyen tek bir id bütün grubu düşürür.
func (s *server) postDecisions(w http.ResponseWriter, r *http.Request) {
	var req bulkDecisionReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be a JSON object with finding_ids as UUIDs")
		return
	}
	if len(req.FindingIDs) == 0 || len(req.FindingIDs) > maxLimit {
		writeErr(w, 422, "INVALID_RESOLUTION", "finding_ids must hold 1..200 ids")
		return
	}
	if msg := decisionErr(req.ResolutionStatus, req.Note); msg != "" {
		writeErr(w, 422, "INVALID_RESOLUTION", msg)
		return
	}
	n, err := s.auditor.Resolve(r.Context(), auth.TenantID(r.Context()), req.FindingIDs, domain.Resolution(req.ResolutionStatus), req.Note, decider(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, bulkDecisionResp{Updated: n})
}

func (s *server) listFindings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repository.FindingFilter{Resolution: q.Get("resolution"), RuleCode: q.Get("rule"), Limit: 50}
	bad := func(msg string) { writeErr(w, 400, "BAD_REQUEST", msg) }
	if f.Resolution != "" && decisionErr(f.Resolution, "") != "" {
		bad("resolution must be OPEN, DISPUTED or APPROVED_EXCEPTION")
		return
	}
	if len(f.RuleCode) > 40 {
		bad("rule is too long")
		return
	}
	if v := q.Get("vendor_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			bad("vendor_id must be a UUID")
			return
		}
		f.VendorID = &id
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			bad("limit must be an integer 1..200")
			return
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			bad("offset must be an integer >= 0")
			return
		}
		f.Offset = n
	}
	var resp findingListResp
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		resp.Items, resp.Total, resp.Counts, err = tx.ListFindings(r.Context(), f)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, resp)
}

func (s *server) getSummary(w http.ResponseWriter, r *http.Request) {
	var sum repository.SummaryRow
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		sum, err = tx.Summary(r.Context())
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, sum)
}
