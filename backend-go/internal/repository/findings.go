package repository

import (
	"context"

	"github.com/google/uuid"
)

// FindingFilter, faturalar arası bulgu listesini daraltır; boş alanlar dikkate alınmaz.
type FindingFilter struct {
	Resolution string
	RuleCode   string
	VendorID   *uuid.UUID
	Limit      int
	Offset     int
}

// FindingListRow, listeden karar verebilmek için yeterli fatura bilgisiyle birlikte tek bir bulgudur.
type FindingListRow struct {
	ID                   uuid.UUID `json:"id"`
	InvoiceID            uuid.UUID `json:"invoice_id"`
	InvoiceNo            string    `json:"invoice_no"`
	VendorName           string    `json:"vendor_name"`
	IssueDate            string    `json:"issue_date"`
	Currency             string    `json:"currency"` // faturanınki
	LineID               *string   `json:"line_id"`
	ItemName             *string   `json:"item_name"`
	Severity             string    `json:"severity"`
	RuleCode             string    `json:"rule_code"`
	Expected             string    `json:"expected"`
	Actual               string    `json:"actual"`
	FinancialImpactMinor int64     `json:"financial_impact_minor"`
	ImpactCurrency       *string   `json:"impact_currency"`
	ResolutionStatus     string    `json:"resolution_status"`
	ResolutionNote       *string   `json:"resolution_note"`
}

// FindingCounts, her kural kodu için eşleşen bulgu sayısıdır.
type FindingCounts map[string]int

// ListFindings bir sayfa (etkisi en büyük olan önce), toplam eşleşme sayısını ve kural başına eşleşme sayısını döner
// (kural sayımında f.RuleCode dikkate alınmaz).
func (tx *Tx) ListFindings(ctx context.Context, f FindingFilter) ([]FindingListRow, int, FindingCounts, error) {
	var res, rule *string
	if f.Resolution != "" {
		res = &f.Resolution
	}
	if f.RuleCode != "" {
		rule = &f.RuleCode
	}
	// shortcut: filtreler tenant'ın bütün bulgularını tarıyor (yalnızca (tenant_id, invoice_id) indeksli); bir tenant 100
	// bin civarı bulguya ulaşınca resolution_status'a indeks ekle.
	const from = ` FROM audit_findings f
		JOIN invoices i ON i.tenant_id=f.tenant_id AND i.id=f.invoice_id
		JOIN vendors v ON v.tenant_id=i.tenant_id AND v.id=i.vendor_id
		LEFT JOIN invoice_lines l ON l.tenant_id=f.tenant_id AND l.id=f.invoice_line_id
		WHERE f.tenant_id=$1 AND ($2::text IS NULL OR f.resolution_status=$2) AND ($3::uuid IS NULL OR i.vendor_id=$3)`
	args := []any{tx.tenantID, res, f.VendorID}

	counts := FindingCounts{}
	crows, err := tx.Query(ctx, `SELECT f.rule_code, count(*)`+from+` GROUP BY 1`, args...)
	if err != nil {
		return nil, 0, nil, wrap("count findings", err)
	}
	total := 0
	for crows.Next() {
		var r string
		var n int
		if err := crows.Scan(&r, &n); err != nil {
			crows.Close()
			return nil, 0, nil, wrap("scan finding count", err)
		}
		counts[r] = n
		if rule == nil || *rule == r {
			total += n
		}
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, 0, nil, wrap("count findings", err)
	}

	rows, err := tx.Query(ctx, `SELECT f.id, i.id, i.invoice_no, v.name, i.issue_date::text, i.currency, l.line_id, l.item_name,
		f.severity, f.rule_code, f.expected_value, f.actual_value, f.financial_impact_minor, f.impact_currency,
		f.resolution_status, f.resolution_note`+from+` AND ($4::text IS NULL OR f.rule_code=$4)
		ORDER BY f.financial_impact_minor DESC, i.issue_date DESC, f.id LIMIT $5 OFFSET $6`,
		append(args, rule, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, nil, wrap("list findings", err)
	}
	defer rows.Close()
	items := []FindingListRow{}
	for rows.Next() {
		var r FindingListRow
		if err := rows.Scan(&r.ID, &r.InvoiceID, &r.InvoiceNo, &r.VendorName, &r.IssueDate, &r.Currency, &r.LineID, &r.ItemName,
			&r.Severity, &r.RuleCode, &r.Expected, &r.Actual, &r.FinancialImpactMinor, &r.ImpactCurrency,
			&r.ResolutionStatus, &r.ResolutionNote); err != nil {
			return nil, 0, nil, wrap("scan finding", err)
		}
		items = append(items, r)
	}
	return items, total, counts, rows.Err()
}

// LockFindingInvoices, verilen bulguların ait olduğu faturaları id sırasıyla kilitler ve onları döner. Faturayı
// ResetAudit gibi önce kilitlemek, kararların yeniden denetimle ya da birbiriyle kilitlenip kalmasını (deadlock) önler.
func (tx *Tx) LockFindingInvoices(ctx context.Context, findingIDs []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT i.id FROM invoices i WHERE i.tenant_id=$1 AND i.id IN
		(SELECT invoice_id FROM audit_findings WHERE tenant_id=$1 AND id = ANY($2)) ORDER BY i.id FOR UPDATE OF i`,
		tx.tenantID, findingIDs)
	if err != nil {
		return nil, wrap("lock finding invoices", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, wrap("scan locked invoice", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
