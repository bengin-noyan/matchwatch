package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// KeptFinding is a finding that survived ResetAudit because a human already acted on it.
type KeptFinding struct {
	RuleCode      domain.RuleCode
	InvoiceLineID *uuid.UUID
	Severity      domain.Severity
	Resolution    domain.Resolution
}

// ResetAudit locks the invoice (serializing concurrent audits), deletes its OPEN findings and its allocations,
// and returns the findings that remain (DISPUTED / APPROVED_EXCEPTION).
func (tx *Tx) ResetAudit(ctx context.Context, invoiceID uuid.UUID) ([]KeptFinding, error) {
	var locked uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM invoices WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tx.tenantID, invoiceID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, wrap("lock invoice", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM audit_findings WHERE tenant_id=$1 AND invoice_id=$2 AND resolution_status='OPEN'`,
		tx.tenantID, invoiceID); err != nil {
		return nil, wrap("delete open findings", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM despatch_allocations WHERE tenant_id=$1 AND invoice_line_id IN
		(SELECT id FROM invoice_lines WHERE tenant_id=$1 AND invoice_id=$2)`, tx.tenantID, invoiceID); err != nil {
		return nil, wrap("delete allocations", err)
	}
	rows, err := tx.Query(ctx, `SELECT rule_code, invoice_line_id, severity, resolution_status FROM audit_findings
		WHERE tenant_id=$1 AND invoice_id=$2`, tx.tenantID, invoiceID)
	if err != nil {
		return nil, wrap("load kept findings", err)
	}
	defer rows.Close()
	var kept []KeptFinding
	for rows.Next() {
		var k KeptFinding
		if err := rows.Scan(&k.RuleCode, &k.InvoiceLineID, &k.Severity, &k.Resolution); err != nil {
			return nil, wrap("scan kept finding", err)
		}
		kept = append(kept, k)
	}
	return kept, rows.Err()
}

// SaveAudit writes findings and allocations, records the matched contract item per invoice line and the status.
func (tx *Tx) SaveAudit(ctx context.Context, invoiceID uuid.UUID, fs []domain.Finding, allocs []domain.Allocation,
	matched map[uuid.UUID]uuid.UUID, status domain.AuditStatus) error {
	for _, f := range fs {
		ev, err := json.Marshal(f.Evidence)
		if err != nil {
			return wrap("marshal evidence", err)
		}
		var cur *string
		if f.ImpactCurrency != "" {
			cur = &f.ImpactCurrency
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_findings(tenant_id, invoice_id, invoice_line_id, severity, rule_code,
			expected_value, actual_value, financial_impact_minor, impact_currency, evidence_json)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, tx.tenantID, invoiceID, f.InvoiceLineID, f.Severity, f.RuleCode,
			f.Expected, f.Actual, f.ImpactMinor, cur, ev); err != nil {
			return wrap("insert finding "+string(f.RuleCode), err)
		}
	}
	for _, a := range allocs {
		if _, err := tx.Exec(ctx, `INSERT INTO despatch_allocations(tenant_id, invoice_line_id, despatch_line_id, qty)
			VALUES ($1,$2,$3,$4)`, tx.tenantID, a.InvoiceLineID, a.DespatchLineID, num(a.Qty)); err != nil {
			return wrap("insert allocation", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE invoice_lines SET matched_contract_item_id=NULL WHERE tenant_id=$1 AND invoice_id=$2`,
		tx.tenantID, invoiceID); err != nil {
		return wrap("clear matched items", err)
	}
	for lineID, itemID := range matched {
		if _, err := tx.Exec(ctx, `UPDATE invoice_lines SET matched_contract_item_id=$1 WHERE tenant_id=$2 AND id=$3 AND invoice_id=$4`,
			itemID, tx.tenantID, lineID, invoiceID); err != nil {
			return wrap("set matched item", err)
		}
	}
	_, err := tx.Exec(ctx, `UPDATE invoices SET audit_status=$1 WHERE tenant_id=$2 AND id=$3`, string(status), tx.tenantID, invoiceID)
	return wrap("set audit status", err)
}
