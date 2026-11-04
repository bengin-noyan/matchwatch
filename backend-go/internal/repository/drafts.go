package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type DraftStatus = string

const (
	DraftExtracting DraftStatus = "EXTRACTING"
	DraftReady      DraftStatus = "READY"
	DraftFailed     DraftStatus = "FAILED"
	DraftApproved   DraftStatus = "APPROVED"
)

var (
	ErrDraftNotReady = errors.New("draft not ready")
	ErrDraftApproved = errors.New("draft already approved")
)

type Draft struct {
	ID                    uuid.UUID
	Status                string
	PDFPath, PDFSHA256    string
	Draft, Warnings, Meta json.RawMessage
	ErrorCode             string
	ContractID            *uuid.UUID
	CreatedAt, UpdatedAt  time.Time
}

func (tx *Tx) InsertDraft(ctx context.Context, id uuid.UUID, pdfPath, sha256hex string) error {
	_, err := tx.Exec(ctx, `INSERT INTO contract_drafts(id, tenant_id, status, pdf_path, pdf_sha256)
		VALUES ($1,$2,'EXTRACTING',$3,$4)`, id, tx.tenantID, pdfPath, sha256hex)
	return wrap("insert draft", err)
}

const draftCols = `id, status, pdf_path, pdf_sha256, draft_json, warnings_json, meta_json, error_code, contract_id, created_at, updated_at`

func (tx *Tx) getDraft(ctx context.Context, id uuid.UUID, suffix string) (Draft, error) {
	var d Draft
	err := tx.QueryRow(ctx, `SELECT `+draftCols+` FROM contract_drafts WHERE id=$1`+suffix, id).Scan(
		&d.ID, &d.Status, &d.PDFPath, &d.PDFSHA256, &d.Draft, &d.Warnings, &d.Meta, &d.ErrorCode,
		&d.ContractID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, wrap("get draft", err)
}

func (tx *Tx) GetDraft(ctx context.Context, id uuid.UUID) (Draft, error) {
	return tx.getDraft(ctx, id, "")
}

func (tx *Tx) LockDraft(ctx context.Context, id uuid.UUID) (Draft, error) {
	return tx.getDraft(ctx, id, " FOR UPDATE")
}

func (tx *Tx) updateDraft(ctx context.Context, id uuid.UUID, set string, args ...any) error {
	tag, err := tx.Exec(ctx, `UPDATE contract_drafts SET `+set+`, updated_at=now() WHERE id=$1`, append([]any{id}, args...)...)
	if err != nil {
		return wrap("update draft", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// updateUnapproved changes extraction state; an APPROVED draft is frozen and yields ErrDraftApproved.
func (tx *Tx) updateUnapproved(ctx context.Context, id uuid.UUID, set string, args ...any) error {
	tag, err := tx.Exec(ctx, `UPDATE contract_drafts SET `+set+`, updated_at=now() WHERE id=$1 AND status <> 'APPROVED'`, append([]any{id}, args...)...)
	if err != nil {
		return wrap("update draft", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	if _, err := tx.GetDraft(ctx, id); err != nil {
		return err // ErrNotFound when the draft does not exist (or belongs to another tenant)
	}
	return ErrDraftApproved
}

func (tx *Tx) SetDraftResult(ctx context.Context, id uuid.UUID, draft, warnings, meta json.RawMessage) error {
	return tx.updateUnapproved(ctx, id, `status='READY', error_code='', draft_json=$2, warnings_json=$3, meta_json=$4`, draft, warnings, meta)
}

func (tx *Tx) SetDraftFailed(ctx context.Context, id uuid.UUID, code string) error {
	return tx.updateUnapproved(ctx, id, `status='FAILED', error_code=$2`, code)
}

func (tx *Tx) SetDraftExtracting(ctx context.Context, id uuid.UUID) error {
	return tx.updateUnapproved(ctx, id, `status='EXTRACTING', error_code=''`)
}

func (tx *Tx) ApproveDraft(ctx context.Context, id, contractID uuid.UUID) error {
	return tx.updateDraft(ctx, id, `status='APPROVED', contract_id=$2`, contractID)
}
