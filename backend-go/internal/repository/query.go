package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// Row types carry JSON tags because the REST layer returns them as is. Decimals are strings, money is kuruş.

type InvoiceFilter struct {
	Status   string
	VendorID *uuid.UUID
	Limit    int
	Offset   int
}

type InvoiceRow struct {
	ID           uuid.UUID `json:"id"`
	VendorID     uuid.UUID `json:"vendor_id"`
	VendorName   string    `json:"vendor_name"`
	ETTN         uuid.UUID `json:"ettn"`
	InvoiceNo    string    `json:"invoice_no"`
	IssueDate    string    `json:"issue_date"`
	DueDate      *string   `json:"due_date"`
	Currency     string    `json:"currency"`
	PayableMinor int64     `json:"payable_minor"`
	AuditStatus  string    `json:"audit_status"`
	OpenFindings int       `json:"open_findings"` // OPEN findings still waiting for a human decision
}

const invoiceCols = `i.id, i.vendor_id, v.name, i.ettn, i.invoice_no, i.issue_date::text, i.due_date::text, i.currency,
	i.payable_minor, i.audit_status, (SELECT count(*) FROM audit_findings f
		WHERE f.tenant_id=i.tenant_id AND f.invoice_id=i.id AND f.resolution_status='OPEN')`
const invoiceFrom = ` FROM invoices i JOIN vendors v ON v.tenant_id=i.tenant_id AND v.id=i.vendor_id`

func scanInvoice(row pgx.Row, r *InvoiceRow) error {
	return row.Scan(&r.ID, &r.VendorID, &r.VendorName, &r.ETTN, &r.InvoiceNo, &r.IssueDate, &r.DueDate, &r.Currency,
		&r.PayableMinor, &r.AuditStatus, &r.OpenFindings)
}

// ListInvoices returns one page (newest first) and the total number of matches.
func (tx *Tx) ListInvoices(ctx context.Context, f InvoiceFilter) ([]InvoiceRow, int, error) {
	// NULL parameters disable a filter, so the SQL text is constant
	var status *string
	if f.Status != "" {
		status = &f.Status
	}
	const where = ` WHERE i.tenant_id=$1 AND ($2::text IS NULL OR i.audit_status=$2) AND ($3::uuid IS NULL OR i.vendor_id=$3)`
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*)`+invoiceFrom+where, tx.tenantID, status, f.VendorID).Scan(&total); err != nil {
		return nil, 0, wrap("count invoices", err)
	}
	rows, err := tx.Query(ctx, `SELECT `+invoiceCols+invoiceFrom+where+` ORDER BY i.created_at DESC, i.id LIMIT $4 OFFSET $5`,
		tx.tenantID, status, f.VendorID, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, wrap("list invoices", err)
	}
	defer rows.Close()
	items := []InvoiceRow{}
	for rows.Next() {
		var r InvoiceRow
		if err := scanInvoice(rows, &r); err != nil {
			return nil, 0, wrap("scan invoice", err)
		}
		items = append(items, r)
	}
	return items, total, rows.Err()
}

type ContractItemRow struct {
	ID               uuid.UUID `json:"id"`
	ContractNo       string    `json:"contract_no"`
	ItemCode         string    `json:"item_code"`
	Name             string    `json:"name"`
	UnitPrice        string    `json:"unit_price"`
	Currency         string    `json:"currency"`
	UOM              string    `json:"uom"`
	SourcePage       int       `json:"source_page"`
	SourceClauseText string    `json:"source_clause_text"`
}

type AllocationRow struct {
	DespatchNo   string `json:"despatch_no"`
	LineID       string `json:"line_id"`
	ItemCode     string `json:"item_code"`
	AllocatedQty string `json:"allocated_qty"`
	DeliveredQty string `json:"delivered_qty"`
	RejectedQty  string `json:"rejected_qty"`
}

type DetailLine struct {
	ID                 uuid.UUID        `json:"id"`
	LineID             string           `json:"line_id"`
	ItemCode           string           `json:"item_code"`
	Barcode            string           `json:"barcode"`
	Name               string           `json:"name"`
	UOM                string           `json:"uom"`
	Qty                string           `json:"qty"`
	UnitPrice          string           `json:"unit_price"`
	AllowanceMinor     int64            `json:"allowance_minor"`
	LineExtensionMinor int64            `json:"line_extension_minor"`
	ContractItem       *ContractItemRow `json:"contract_item"`
	DespatchLines      []AllocationRow  `json:"despatch_lines"`
}

type FindingRow struct {
	ID                   uuid.UUID       `json:"id"`
	LineID               *string         `json:"line_id"` // invoice line_id (cbc:ID), null for header findings
	Severity             string          `json:"severity"`
	RuleCode             string          `json:"rule_code"`
	Expected             string          `json:"expected"`
	Actual               string          `json:"actual"`
	FinancialImpactMinor int64           `json:"financial_impact_minor"`
	ImpactCurrency       *string         `json:"impact_currency"`
	Evidence             json.RawMessage `json:"evidence"`
	ResolutionStatus     string          `json:"resolution_status"`
	ResolutionNote       *string         `json:"resolution_note"`
}

type InvoiceDetailRow struct {
	InvoiceRow
	Lines    []DetailLine `json:"lines"`
	Findings []FindingRow `json:"findings"`
}

// InvoiceDetail returns the invoice with, per line, the matched contract item and the despatch lines it draws on.
func (tx *Tx) InvoiceDetail(ctx context.Context, id uuid.UUID) (InvoiceDetailRow, error) {
	var d InvoiceDetailRow
	err := scanInvoice(tx.QueryRow(ctx, `SELECT `+invoiceCols+invoiceFrom+` WHERE i.tenant_id=$1 AND i.id=$2`, tx.tenantID, id), &d.InvoiceRow)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, wrap("invoice detail", err)
	}

	rows, err := tx.Query(ctx, `SELECT l.id, l.line_id, l.item_code, l.barcode, l.item_name, l.uom, l.qty::text, l.unit_price::text,
		l.allowance_amount_minor, l.line_extension_minor,
		ci.id, c.contract_no, ci.item_code, ci.item_name, ci.unit_price::text, ci.currency, ci.uom, ci.source_page, ci.source_clause_text
		FROM invoice_lines l
		LEFT JOIN contract_items ci ON ci.tenant_id=l.tenant_id AND ci.id=l.matched_contract_item_id
		LEFT JOIN contracts c ON c.tenant_id=ci.tenant_id AND c.id=ci.contract_id
		WHERE l.tenant_id=$1 AND l.invoice_id=$2 ORDER BY length(l.line_id), l.line_id, l.id`, tx.tenantID, id)
	if err != nil {
		return d, wrap("detail lines", err)
	}
	defer rows.Close()
	d.Lines = []DetailLine{}
	byID := map[uuid.UUID]int{}
	for rows.Next() {
		l := DetailLine{DespatchLines: []AllocationRow{}}
		var ciID *uuid.UUID
		var ci ContractItemRow
		var cNo, cCode, cName, cPrice, cCur, cUOM, cClause *string
		var cPage *int
		if err := rows.Scan(&l.ID, &l.LineID, &l.ItemCode, &l.Barcode, &l.Name, &l.UOM, &l.Qty, &l.UnitPrice,
			&l.AllowanceMinor, &l.LineExtensionMinor, &ciID, &cNo, &cCode, &cName, &cPrice, &cCur, &cUOM, &cPage, &cClause); err != nil {
			return d, wrap("scan detail line", err)
		}
		if ciID != nil {
			ci = ContractItemRow{ID: *ciID, ContractNo: *cNo, ItemCode: *cCode, Name: *cName, UnitPrice: *cPrice,
				Currency: *cCur, UOM: *cUOM, SourcePage: *cPage, SourceClauseText: *cClause}
			l.ContractItem = &ci
		}
		byID[l.ID] = len(d.Lines)
		d.Lines = append(d.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return d, err
	}
	rows.Close()

	arows, err := tx.Query(ctx, `SELECT a.invoice_line_id, da.despatch_no, dl.line_id, dl.item_code, a.qty::text,
		dl.delivered_qty::text, dl.rejected_qty::text
		FROM despatch_allocations a
		JOIN invoice_lines l ON l.tenant_id=a.tenant_id AND l.id=a.invoice_line_id
		JOIN despatch_lines dl ON dl.tenant_id=a.tenant_id AND dl.id=a.despatch_line_id
		JOIN despatch_advices da ON da.tenant_id=dl.tenant_id AND da.id=dl.despatch_id
		WHERE a.tenant_id=$1 AND l.invoice_id=$2 ORDER BY da.despatch_no, length(dl.line_id), dl.line_id`, tx.tenantID, id)
	if err != nil {
		return d, wrap("detail allocations", err)
	}
	defer arows.Close()
	for arows.Next() {
		var lineID uuid.UUID
		var a AllocationRow
		if err := arows.Scan(&lineID, &a.DespatchNo, &a.LineID, &a.ItemCode, &a.AllocatedQty, &a.DeliveredQty, &a.RejectedQty); err != nil {
			return d, wrap("scan allocation", err)
		}
		i := byID[lineID]
		d.Lines[i].DespatchLines = append(d.Lines[i].DespatchLines, a)
	}
	if err := arows.Err(); err != nil {
		return d, err
	}
	arows.Close()

	frows, err := tx.Query(ctx, `SELECT f.id, l.line_id, f.severity, f.rule_code, f.expected_value, f.actual_value,
		f.financial_impact_minor, f.impact_currency, f.evidence_json, f.resolution_status, f.resolution_note
		FROM audit_findings f LEFT JOIN invoice_lines l ON l.tenant_id=f.tenant_id AND l.id=f.invoice_line_id
		WHERE f.tenant_id=$1 AND f.invoice_id=$2 ORDER BY f.rule_code, length(l.line_id), l.line_id, f.id`, tx.tenantID, id)
	if err != nil {
		return d, wrap("detail findings", err)
	}
	defer frows.Close()
	d.Findings = []FindingRow{}
	for frows.Next() {
		var f FindingRow
		if err := frows.Scan(&f.ID, &f.LineID, &f.Severity, &f.RuleCode, &f.Expected, &f.Actual, &f.FinancialImpactMinor,
			&f.ImpactCurrency, &f.Evidence, &f.ResolutionStatus, &f.ResolutionNote); err != nil {
			return d, wrap("scan finding", err)
		}
		d.Findings = append(d.Findings, f)
	}
	return d, frows.Err()
}

// UpdateFinding records a human resolution and returns the finding's invoice; status must already be validated by the caller.
func (tx *Tx) UpdateFinding(ctx context.Context, id uuid.UUID, status domain.Resolution, note string) (uuid.UUID, error) {
	var n *string
	if note != "" {
		n = &note
	}
	var invoiceID uuid.UUID
	err := tx.QueryRow(ctx, `UPDATE audit_findings SET resolution_status=$1, resolution_note=$2, updated_at=now()
		WHERE tenant_id=$3 AND id=$4 RETURNING invoice_id`, string(status), n, tx.tenantID, id).Scan(&invoiceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return invoiceID, ErrNotFound
	}
	return invoiceID, wrap("update finding", err)
}

// CountedSeverities lists the severities of the invoice's findings that still count towards its status
// (everything except APPROVED_EXCEPTION).
func (tx *Tx) CountedSeverities(ctx context.Context, invoiceID uuid.UUID) ([]domain.Severity, error) {
	rows, err := tx.Query(ctx, `SELECT severity FROM audit_findings
		WHERE tenant_id=$1 AND invoice_id=$2 AND resolution_status <> 'APPROVED_EXCEPTION'`, tx.tenantID, invoiceID)
	if err != nil {
		return nil, wrap("counted severities", err)
	}
	defer rows.Close()
	var out []domain.Severity
	for rows.Next() {
		var sev domain.Severity
		if err := rows.Scan(&sev); err != nil {
			return nil, wrap("scan severity", err)
		}
		out = append(out, sev)
	}
	return out, rows.Err()
}

// SetAuditStatus stores the invoice's audit status.
func (tx *Tx) SetAuditStatus(ctx context.Context, invoiceID uuid.UUID, status domain.AuditStatus) error {
	_, err := tx.Exec(ctx, `UPDATE invoices SET audit_status=$1 WHERE tenant_id=$2 AND id=$3`, string(status), tx.tenantID, invoiceID)
	return wrap("set audit status", err)
}

type CurrencySummary struct {
	Currency      string `json:"currency"`
	CaughtMinor   int64  `json:"caught_minor"`   // every finding except APPROVED_EXCEPTION
	DisputedMinor int64  `json:"disputed_minor"` // DISPUTED only
	OpenCount     int    `json:"open_count"`
	DisputedCount int    `json:"disputed_count"`
}

type SummaryRow struct {
	ByCurrency []CurrencySummary `json:"by_currency"`
	Invoices   map[string]int    `json:"invoices"`
}

func (tx *Tx) Summary(ctx context.Context) (SummaryRow, error) {
	s := SummaryRow{ByCurrency: []CurrencySummary{}, Invoices: map[string]int{"GREEN": 0, "YELLOW": 0, "RED": 0, "PENDING": 0}}
	rows, err := tx.Query(ctx, `SELECT COALESCE(f.impact_currency, i.currency) AS cur,
		COALESCE(SUM(f.financial_impact_minor) FILTER (WHERE f.resolution_status <> 'APPROVED_EXCEPTION'), 0)::bigint,
		COALESCE(SUM(f.financial_impact_minor) FILTER (WHERE f.resolution_status = 'DISPUTED'), 0)::bigint,
		count(*) FILTER (WHERE f.resolution_status = 'OPEN'),
		count(*) FILTER (WHERE f.resolution_status = 'DISPUTED')
		FROM audit_findings f JOIN invoices i ON i.tenant_id=f.tenant_id AND i.id=f.invoice_id
		WHERE f.tenant_id=$1 GROUP BY 1 ORDER BY 1`, tx.tenantID)
	if err != nil {
		return s, wrap("summary findings", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c CurrencySummary
		if err := rows.Scan(&c.Currency, &c.CaughtMinor, &c.DisputedMinor, &c.OpenCount, &c.DisputedCount); err != nil {
			return s, wrap("scan summary", err)
		}
		s.ByCurrency = append(s.ByCurrency, c)
	}
	if err := rows.Err(); err != nil {
		return s, err
	}
	rows.Close()
	irows, err := tx.Query(ctx, `SELECT audit_status, count(*) FROM invoices WHERE tenant_id=$1 GROUP BY 1`, tx.tenantID)
	if err != nil {
		return s, wrap("summary invoices", err)
	}
	defer irows.Close()
	for irows.Next() {
		var st string
		var n int
		if err := irows.Scan(&st, &n); err != nil {
			return s, wrap("scan summary invoices", err)
		}
		s.Invoices[st] = n
	}
	return s, irows.Err()
}
