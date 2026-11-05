package transporthttp

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

type idResp struct {
	ID uuid.UUID `json:"id"`
}

type ingestResp struct {
	ID        uuid.UUID   `json:"id"`
	Reaudited []uuid.UUID `json:"reaudited"`
}

type receiptResp struct {
	DespatchNo string      `json:"despatch_no"`
	Reaudited  []uuid.UUID `json:"reaudited"`
}

type invoiceResp struct {
	ID          uuid.UUID `json:"id"`
	AuditStatus string    `json:"audit_status"`
}

type invoiceListResp struct {
	Items  []repository.InvoiceRow  `json:"items"`
	Total  int                      `json:"total"`
	Counts repository.InvoiceCounts `json:"counts"` // per audit status, ignoring the status filter
}

type patchReq struct {
	ResolutionStatus string `json:"resolution_status"`
	Note             string `json:"note"`
}

type patchResp struct {
	ID               uuid.UUID `json:"id"`
	ResolutionStatus string    `json:"resolution_status"`
}

type draftResp struct {
	ID         uuid.UUID       `json:"id"`
	Status     string          `json:"status"`
	Draft      json.RawMessage `json:"draft"`
	Warnings   json.RawMessage `json:"warnings"`
	Meta       json.RawMessage `json:"meta"`
	ErrorCode  string          `json:"error_code,omitempty"`
	ContractID *uuid.UUID      `json:"contract_id,omitempty"`
}
