package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// Satır tiplerinde JSON etiketleri var, çünkü REST katmanı onları olduğu gibi döner. Ondalıklar string, para kuruş
// cinsinden.

type InvoiceFilter struct {
	Status   string
	VendorID *uuid.UUID
	Query    string  // fatura no, tedarikçi adı ya da ETTN içinde geçen metin; büyük-küçük harf ve Türkçe i'ler fark etmez
	From, To *string // düzenleme tarihi aralığı, YYYY-MM-DD, uçlar dahil
	OpenOnly bool    // yalnızca hâlâ karar bekleyen bulgusu olan faturalar
	Sort     string  // invoiceSorts'taki anahtarlardan biri; "" ise en son yüklenen önce
	Desc     bool
	Limit    int
	Offset   int
}

// InvoiceCounts, denetim durumuna göre eşleşme sayılarını verir. Durum filtresini dikkate almaz, böylece durum çipleri
// sayıları gösterebilir.
type InvoiceCounts map[string]int

type InvoiceRow struct {
	ID             uuid.UUID `json:"id"`
	VendorID       uuid.UUID `json:"vendor_id"`
	VendorName     string    `json:"vendor_name"`
	ETTN           uuid.UUID `json:"ettn"`
	InvoiceNo      string    `json:"invoice_no"`
	IssueDate      string    `json:"issue_date"`
	DueDate        *string   `json:"due_date"`
	Currency       string    `json:"currency"`
	PayableMinor   int64     `json:"payable_minor"`
	AuditStatus    string    `json:"audit_status"`
	ReauditPending bool      `json:"reaudit_pending"` // reaudit_since dolu: durum henüz güncel olmayabilir
	OpenFindings   int       `json:"open_findings"`   // hâlâ bir insan kararı bekleyen OPEN bulgular
}

const openFindings = `(SELECT count(*) FROM audit_findings f
		WHERE f.tenant_id=i.tenant_id AND f.invoice_id=i.id AND f.resolution_status='OPEN')`
const invoiceCols = `i.id, i.vendor_id, v.name, i.ettn, i.invoice_no, i.issue_date::text, i.due_date::text, i.currency,
	i.payable_minor, i.audit_status, i.reaudit_since IS NOT NULL, ` + openFindings

// invoiceSorts, istemcinin seçebileceği ORDER BY ifadelerinin izinli listesidir.
var invoiceSorts = map[string]string{
	"issue_date":    "i.issue_date",
	"invoice_no":    "i.invoice_no",
	"vendor":        "v.name",
	"payable":       "i.payable_minor",
	"status":        "CASE i.audit_status WHEN 'RED' THEN 0 WHEN 'YELLOW' THEN 1 WHEN 'PENDING' THEN 2 ELSE 3 END",
	"open_findings": openFindings,
}

// ValidInvoiceSort, key'in InvoiceFilter.Sort olarak verilip verilemeyeceğini söyler.
func ValidInvoiceSort(key string) bool { _, ok := invoiceSorts[key]; return ok }

// likeEscape, s'nin LIKE kalıbı içinde harfi harfine eşleşmesini sağlar (varsayılan kaçış karakteri ters bölü).
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

const invoiceFrom = ` FROM invoices i JOIN vendors v ON v.tenant_id=i.tenant_id AND v.id=i.vendor_id`

func scanInvoice(row pgx.Row, r *InvoiceRow) error {
	return row.Scan(&r.ID, &r.VendorID, &r.VendorName, &r.ETTN, &r.InvoiceNo, &r.IssueDate, &r.DueDate, &r.Currency,
		&r.PayableMinor, &r.AuditStatus, &r.ReauditPending, &r.OpenFindings)
}

// ListInvoices returns one page, the number of matches and the matches per status (ignoring f.Status).
func (tx *Tx) ListInvoices(ctx context.Context, f InvoiceFilter) ([]InvoiceRow, int, InvoiceCounts, error) {
	// NULL parameters disable a filter, so the SQL text only varies by the whitelisted sort
	var query, status *string
	if f.Query != "" {
		q := likeEscape(f.Query)
		query = &q
	}
	if f.Status != "" {
		status = &f.Status
	}
	// shortcut: the search scans the tenant's invoices; add a pg_trgm index once tenants hold ~100k invoices.
	const where = ` WHERE i.tenant_id=$1 AND ($2::uuid IS NULL OR i.vendor_id=$2)
		AND ($3::text IS NULL OR translate(lower(i.invoice_no||' '||v.name||' '||i.ettn::text), 'ı', 'i')
			LIKE '%'||translate(lower($3), 'ı', 'i')||'%')
		AND ($4::date IS NULL OR i.issue_date >= $4) AND ($5::date IS NULL OR i.issue_date <= $5)
		AND (NOT $6 OR ` + openFindings + ` > 0)`
	args := []any{tx.tenantID, f.VendorID, query, f.From, f.To, f.OpenOnly}

	counts := InvoiceCounts{}
	crows, err := tx.Query(ctx, `SELECT i.audit_status, count(*)`+invoiceFrom+where+` GROUP BY 1`, args...)
	if err != nil {
		return nil, 0, nil, wrap("count invoices", err)
	}
	total := 0
	for crows.Next() {
		var st string
		var n int
		if err := crows.Scan(&st, &n); err != nil {
			crows.Close()
			return nil, 0, nil, wrap("scan count", err)
		}
		counts[st] = n
		if status == nil || *status == st {
			total += n
		}
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, 0, nil, wrap("count invoices", err)
	}

	order := "i.created_at DESC"
	if e, ok := invoiceSorts[f.Sort]; ok {
		order = e + map[bool]string{false: " ASC", true: " DESC"}[f.Desc]
	}
	rows, err := tx.Query(ctx, `SELECT `+invoiceCols+invoiceFrom+where+` AND ($7::text IS NULL OR i.audit_status=$7)
		ORDER BY `+order+`, i.id LIMIT $8 OFFSET $9`, append(args, status, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, nil, wrap("list invoices", err)
	}
	defer rows.Close()
	items := []InvoiceRow{}
	for rows.Next() {
		var r InvoiceRow
		if err := scanInvoice(rows, &r); err != nil {
			return nil, 0, nil, wrap("scan invoice", err)
		}
		items = append(items, r)
	}
	return items, total, counts, rows.Err()
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
	Decisions            []DecisionRow   `json:"decisions"` // oldest first

	lineUUID *uuid.UUID // identity for matching decisions
}

// DecisionRow is one human decision on a finding.
type DecisionRow struct {
	ResolutionStatus string    `json:"resolution_status"`
	Note             *string   `json:"note"`
	DecidedByEmail   *string   `json:"decided_by_email"` // null for dev tokens
	DecidedAt        time.Time `json:"decided_at"`
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

	frows, err := tx.Query(ctx, `SELECT f.id, f.invoice_line_id, l.line_id, f.severity, f.rule_code, f.expected_value, f.actual_value,
		f.financial_impact_minor, f.impact_currency, f.evidence_json, f.resolution_status, f.resolution_note
		FROM audit_findings f LEFT JOIN invoice_lines l ON l.tenant_id=f.tenant_id AND l.id=f.invoice_line_id
		WHERE f.tenant_id=$1 AND f.invoice_id=$2 ORDER BY f.rule_code, length(l.line_id), l.line_id, f.id`, tx.tenantID, id)
	if err != nil {
		return d, wrap("detail findings", err)
	}
	defer frows.Close()
	d.Findings = []FindingRow{}
	for frows.Next() {
		f := FindingRow{Decisions: []DecisionRow{}}
		if err := frows.Scan(&f.ID, &f.lineUUID, &f.LineID, &f.Severity, &f.RuleCode, &f.Expected, &f.Actual, &f.FinancialImpactMinor,
			&f.ImpactCurrency, &f.Evidence, &f.ResolutionStatus, &f.ResolutionNote); err != nil {
			return d, wrap("scan finding", err)
		}
		d.Findings = append(d.Findings, f)
	}
	if err := frows.Err(); err != nil {
		return d, err
	}
	frows.Close()
	return d, tx.attachDecisions(ctx, id, d.Findings)
}

// attachDecisions adds each finding's trail, matched by (line, rule) like ResetAudit's kept findings.
func (tx *Tx) attachDecisions(ctx context.Context, invoiceID uuid.UUID, fs []FindingRow) error {
	rows, err := tx.Query(ctx, `SELECT invoice_line_id, rule_code, resolution_status, note, decided_by_email, decided_at
		FROM finding_decisions WHERE tenant_id=$1 AND invoice_id=$2 ORDER BY decided_at, id`, tx.tenantID, invoiceID)
	if err != nil {
		return wrap("finding decisions", err)
	}
	defer rows.Close()
	for rows.Next() {
		var line *uuid.UUID
		var rule string
		var dr DecisionRow
		if err := rows.Scan(&line, &rule, &dr.ResolutionStatus, &dr.Note, &dr.DecidedByEmail, &dr.DecidedAt); err != nil {
			return wrap("scan decision", err)
		}
		for i := range fs {
			f := &fs[i]
			if f.RuleCode == rule && (f.lineUUID == nil && line == nil || f.lineUUID != nil && line != nil && *f.lineUUID == *line) {
				f.Decisions = append(f.Decisions, dr)
			}
		}
	}
	return rows.Err()
}

// UpdateFinding records a human resolution, appends it to finding_decisions and returns the finding's invoice;
// status must already be validated by the caller. by is the deciding user (nil for dev tokens).
func (tx *Tx) UpdateFinding(ctx context.Context, id uuid.UUID, status domain.Resolution, note string, by *uuid.UUID) (uuid.UUID, error) {
	var n *string
	if note != "" {
		n = &note
	}
	var invoiceID uuid.UUID
	err := tx.QueryRow(ctx, `WITH f AS (
			UPDATE audit_findings SET resolution_status=$1, resolution_note=$2, updated_at=now()
			WHERE tenant_id=$3 AND id=$4 RETURNING invoice_id, invoice_line_id, rule_code)
		INSERT INTO finding_decisions(tenant_id, invoice_id, invoice_line_id, rule_code, resolution_status, note,
			decided_by, decided_by_email)
		SELECT $3, f.invoice_id, f.invoice_line_id, f.rule_code, $1, $2, $5::uuid, (SELECT email FROM auth_user($5::uuid, $3))
		FROM f RETURNING invoice_id`, string(status), n, tx.tenantID, id, by).Scan(&invoiceID)
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

// ContractFilePath returns where the approved PDF of contract no is stored; ErrNotFound when the contract is
// unknown or was entered without a PDF.
func (tx *Tx) ContractFilePath(ctx context.Context, no string) (string, error) {
	var p *string
	err := tx.QueryRow(ctx, `SELECT file_path FROM contracts WHERE tenant_id=$1 AND contract_no=$2`, tx.tenantID, no).Scan(&p)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (p == nil || *p == "")) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", wrap("contract file path", err)
	}
	return *p, nil
}
