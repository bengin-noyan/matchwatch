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

// fail maps repository errors to HTTP; anything unknown is logged and answered with a generic 500.
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
	case errors.Is(err, repository.ErrNotFound):
		writeErr(w, 404, "NOT_FOUND", "not found")
	case errors.As(err, &pgErr) && pgErr.Code == "23514": // CHECK, e.g. rejected_qty > delivered_qty
		writeErr(w, 422, "INVALID_DOCUMENT", "document violates a data constraint")
	default:
		log.Printf("internal error: %v", err)
		writeErr(w, 500, "INTERNAL", "internal error")
	}
}

// readXML reads the capped body; it answers 413/400 itself and reports ok=false.
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

// rawPath is built only from the tenant UUID and the canonical ETTN, never from client text.
func (s *server) rawPath(tid uuid.UUID, ettn string) string {
	return filepath.Join(s.storageDir, tid.String(), ettn+".xml")
}

func writeRaw(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}

// ingest runs fn in the tenant tx. fn calls write() only after the DB accepted the row (so duplicates and
// mismatches never touch the disk); if the commit then fails, the file is removed again.
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
		var err error
		if doc.VendorID, err = tx.UpsertVendor(r.Context(), doc.VendorVKN, name); err != nil {
			return err
		}
		id, err = tx.InsertContract(r.Context(), doc.ContractInput)
		return err
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, idResp{ID: id})
}

// reaudit re-runs the audit of every invoice that references a newly ingested despatch.
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
	err = s.ingest(ctx, tid, s.rawPath(tid, d.ETTN), body, func(tx *repository.Tx, write func() error) error {
		if err := checkCustomer(ctx, tx, d.Customer.VKN); err != nil {
			return err
		}
		v, err := tx.UpsertVendor(ctx, d.Supplier.VKN, d.Supplier.Title)
		if err != nil {
			return err
		}
		if id, err = tx.InsertDespatch(ctx, v, d, s.rawPath(tid, d.ETTN)); err != nil {
			return err
		}
		if refs, err = tx.InvoicesReferencingDespatch(ctx, v, d.Number); err != nil {
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
		v, err := tx.UpsertVendor(ctx, rc.Supplier.VKN, rc.Supplier.Title) // rolled back if the despatch is unknown
		if err != nil {
			return err
		}
		if no, err = tx.ApplyReceipt(ctx, v, rc); err != nil {
			return err
		}
		refs, err = tx.InvoicesReferencingDespatch(ctx, v, no)
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
	err = s.ingest(ctx, tid, s.rawPath(tid, inv.ETTN), body, func(tx *repository.Tx, write func() error) error {
		if err := checkCustomer(ctx, tx, inv.Customer.VKN); err != nil {
			return err
		}
		v, err := tx.UpsertVendor(ctx, inv.Supplier.VKN, inv.Supplier.Title)
		if err != nil {
			return err
		}
		if id, err = tx.InsertInvoice(ctx, v, inv, s.rawPath(tid, inv.ETTN)); err != nil {
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
	var items []repository.InvoiceRow
	var total int
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		items, total, err = tx.ListInvoices(r.Context(), f)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, listResp{Items: items, Total: total})
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
	switch st := domain.Resolution(req.ResolutionStatus); st {
	case domain.ResolutionOpen, domain.ResolutionDisputed, domain.ResolutionApprovedException:
	default:
		writeErr(w, 422, "INVALID_RESOLUTION", "resolution_status must be OPEN, DISPUTED or APPROVED_EXCEPTION")
		return
	}
	if len(req.Note) > maxNoteLen {
		writeErr(w, 422, "INVALID_RESOLUTION", "note exceeds 2000 bytes")
		return
	}
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) error {
		return tx.UpdateFinding(r.Context(), id, domain.Resolution(req.ResolutionStatus), req.Note)
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, patchResp{ID: id, ResolutionStatus: req.ResolutionStatus})
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
