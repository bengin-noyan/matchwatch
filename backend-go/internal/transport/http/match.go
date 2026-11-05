package transporthttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// postLineMatch records a human-confirmed alias (D16): this line's item name means that contract item, for this
// vendor from now on. The invoice is then re-audited.
func (s *server) postLineMatch(w http.ResponseWriter, r *http.Request) {
	lineID, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	var req struct {
		ContractItemID string `json:"contract_item_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be a JSON object")
		return
	}
	itemID, err := uuid.Parse(req.ContractItemID)
	if err != nil {
		writeErr(w, 400, "BAD_REQUEST", "contract_item_id must be a UUID")
		return
	}
	tid := auth.TenantID(r.Context())
	var invoiceID uuid.UUID
	err = s.store.InTenant(r.Context(), tid, func(tx *repository.Tx) error {
		invoiceID, err = tx.UpsertAlias(r.Context(), lineID, itemID)
		return err
	})
	switch {
	case errors.Is(err, repository.ErrVendorMismatch):
		writeErr(w, 422, "VENDOR_MISMATCH", "contract item belongs to another vendor")
		return
	case errors.Is(err, repository.ErrEmptyItemName):
		writeErr(w, 422, "EMPTY_ITEM_NAME", "invoice line has no usable item name")
		return
	case err != nil:
		fail(w, err)
		return
	}
	st, err := s.auditor.AuditInvoice(r.Context(), tid, invoiceID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"invoice_id": invoiceID.String(), "audit_status": string(st)})
}

func (s *server) listAliases(w http.ResponseWriter, r *http.Request) {
	var vendorID *uuid.UUID
	if v := r.URL.Query().Get("vendor_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, 400, "BAD_REQUEST", "vendor_id must be a UUID")
			return
		}
		vendorID = &id
	}
	var items []repository.AliasRow
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		items, err = tx.ListAliases(r.Context(), vendorID)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// deleteAlias undoes a wrong item match and re-audits every invoice that relied on it.
func (s *server) deleteAlias(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	tid := auth.TenantID(r.Context())
	var invoices []uuid.UUID
	err := s.store.InTenant(r.Context(), tid, func(tx *repository.Tx) (err error) {
		invoices, err = tx.DeleteAlias(r.Context(), id)
		return
	})
	if err != nil {
		fail(w, err)
		return
	}
	// shortcut: re-audits run after the delete commits, one by one; a failure leaves the rest as they were
	// (recover with POST /invoices/{id}/audit), same as an ingest whose audit fails.
	reaudited := []uuid.UUID{}
	for _, inv := range invoices {
		if _, err := s.auditor.AuditInvoice(r.Context(), tid, inv); err != nil {
			fail(w, err)
			return
		}
		reaudited = append(reaudited, inv)
	}
	writeJSON(w, 200, map[string]any{"reaudited": reaudited})
}
