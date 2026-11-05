package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// "Caught" everywhere below means every finding except APPROVED_EXCEPTION, as in Summary.
// The period filters on the invoice's issue date; nil bounds are open.

type WeekPoint struct {
	Week        string `json:"week"` // Monday of the ISO week, YYYY-MM-DD
	Currency    string `json:"currency"`
	CaughtMinor int64  `json:"caught_minor"`
}

type RuleTotal struct {
	RuleCode    string `json:"rule_code"`
	Currency    string `json:"currency"`
	ImpactMinor int64  `json:"impact_minor"`
	Count       int    `json:"count"`
}

type VendorTotal struct {
	VendorID    uuid.UUID `json:"vendor_id"`
	VendorName  string    `json:"vendor_name"`
	Currency    string    `json:"currency"`
	ImpactMinor int64     `json:"impact_minor"`
	Count       int       `json:"count"`
}

type OpenFinding struct {
	FindingID   uuid.UUID `json:"finding_id"`
	InvoiceID   uuid.UUID `json:"invoice_id"`
	InvoiceNo   string    `json:"invoice_no"`
	VendorName  string    `json:"vendor_name"`
	IssueDate   string    `json:"issue_date"`
	RuleCode    string    `json:"rule_code"`
	LineID      *string   `json:"line_id"`
	ImpactMinor int64     `json:"impact_minor"`
	Currency    string    `json:"currency"`
}

type Dashboard struct {
	ByCurrency []CurrencySummary `json:"by_currency"`
	Invoices   map[string]int    `json:"invoices"`
	Weekly     []WeekPoint       `json:"weekly"`
	ByRule     []RuleTotal       `json:"by_rule"`
	ByVendor   []VendorTotal     `json:"by_vendor"`
	OpenTop    []OpenFinding     `json:"open_top"`
}

const dashFindings = ` FROM audit_findings f JOIN invoices i ON i.tenant_id=f.tenant_id AND i.id=f.invoice_id
	WHERE f.tenant_id=$1 AND ($2::date IS NULL OR i.issue_date >= $2) AND ($3::date IS NULL OR i.issue_date <= $3)`

const findingCur = `COALESCE(f.impact_currency, i.currency)`

func (tx *Tx) Dashboard(ctx context.Context, from, to *time.Time) (Dashboard, error) {
	d := Dashboard{ByCurrency: []CurrencySummary{}, Invoices: map[string]int{"GREEN": 0, "YELLOW": 0, "RED": 0, "PENDING": 0},
		Weekly: []WeekPoint{}, ByRule: []RuleTotal{}, ByVendor: []VendorTotal{}, OpenTop: []OpenFinding{}}
	args := []any{tx.tenantID, from, to}

	rows, err := tx.Query(ctx, `SELECT `+findingCur+`,
		COALESCE(SUM(f.financial_impact_minor) FILTER (WHERE f.resolution_status <> 'APPROVED_EXCEPTION'), 0)::bigint,
		COALESCE(SUM(f.financial_impact_minor) FILTER (WHERE f.resolution_status = 'DISPUTED'), 0)::bigint,
		count(*) FILTER (WHERE f.resolution_status = 'OPEN'), count(*) FILTER (WHERE f.resolution_status = 'DISPUTED')`+
		dashFindings+` GROUP BY 1 ORDER BY 2 DESC, 1`, args...)
	if err != nil {
		return d, wrap("dashboard currencies", err)
	}
	for rows.Next() {
		var c CurrencySummary
		if err := rows.Scan(&c.Currency, &c.CaughtMinor, &c.DisputedMinor, &c.OpenCount, &c.DisputedCount); err != nil {
			rows.Close()
			return d, wrap("scan dashboard currency", err)
		}
		d.ByCurrency = append(d.ByCurrency, c)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT audit_status, count(*) FROM invoices i
		WHERE tenant_id=$1 AND ($2::date IS NULL OR i.issue_date >= $2) AND ($3::date IS NULL OR i.issue_date <= $3) GROUP BY 1`, args...)
	if err != nil {
		return d, wrap("dashboard invoices", err)
	}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return d, wrap("scan dashboard invoices", err)
		}
		d.Invoices[st] = n
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT date_trunc('week', i.issue_date)::date::text, `+findingCur+`,
		COALESCE(SUM(f.financial_impact_minor), 0)::bigint`+dashFindings+` AND f.resolution_status <> 'APPROVED_EXCEPTION'
		GROUP BY 1, 2 ORDER BY 1, 2`, args...)
	if err != nil {
		return d, wrap("dashboard weekly", err)
	}
	for rows.Next() {
		var p WeekPoint
		if err := rows.Scan(&p.Week, &p.Currency, &p.CaughtMinor); err != nil {
			rows.Close()
			return d, wrap("scan weekly", err)
		}
		d.Weekly = append(d.Weekly, p)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT f.rule_code, `+findingCur+`, COALESCE(SUM(f.financial_impact_minor), 0)::bigint, count(*)`+
		dashFindings+` AND f.resolution_status <> 'APPROVED_EXCEPTION' GROUP BY 1, 2 ORDER BY 3 DESC, 4 DESC, 1`, args...)
	if err != nil {
		return d, wrap("dashboard rules", err)
	}
	for rows.Next() {
		var r RuleTotal
		if err := rows.Scan(&r.RuleCode, &r.Currency, &r.ImpactMinor, &r.Count); err != nil {
			rows.Close()
			return d, wrap("scan rules", err)
		}
		d.ByRule = append(d.ByRule, r)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT v.id, v.name, `+findingCur+`, COALESCE(SUM(f.financial_impact_minor), 0)::bigint, count(*)`+
		` FROM audit_findings f JOIN invoices i ON i.tenant_id=f.tenant_id AND i.id=f.invoice_id
		JOIN vendors v ON v.tenant_id=i.tenant_id AND v.id=i.vendor_id
		WHERE f.tenant_id=$1 AND ($2::date IS NULL OR i.issue_date >= $2) AND ($3::date IS NULL OR i.issue_date <= $3)
		AND f.resolution_status <> 'APPROVED_EXCEPTION' GROUP BY 1, 2, 3 ORDER BY 4 DESC, 2 LIMIT 100`, args...)
	if err != nil {
		return d, wrap("dashboard vendors", err)
	}
	for rows.Next() {
		var v VendorTotal
		if err := rows.Scan(&v.VendorID, &v.VendorName, &v.Currency, &v.ImpactMinor, &v.Count); err != nil {
			rows.Close()
			return d, wrap("scan vendors", err)
		}
		d.ByVendor = append(d.ByVendor, v)
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT f.id, i.id, i.invoice_no, v.name, i.issue_date::text, f.rule_code, l.line_id,
		f.financial_impact_minor, `+findingCur+`
		FROM audit_findings f JOIN invoices i ON i.tenant_id=f.tenant_id AND i.id=f.invoice_id
		JOIN vendors v ON v.tenant_id=i.tenant_id AND v.id=i.vendor_id
		LEFT JOIN invoice_lines l ON l.tenant_id=f.tenant_id AND l.id=f.invoice_line_id
		WHERE f.tenant_id=$1 AND ($2::date IS NULL OR i.issue_date >= $2) AND ($3::date IS NULL OR i.issue_date <= $3)
		AND f.resolution_status = 'OPEN' ORDER BY f.financial_impact_minor DESC, i.issue_date DESC, f.id LIMIT 8`, args...)
	if err != nil {
		return d, wrap("dashboard open", err)
	}
	defer rows.Close()
	for rows.Next() {
		var o OpenFinding
		if err := rows.Scan(&o.FindingID, &o.InvoiceID, &o.InvoiceNo, &o.VendorName, &o.IssueDate, &o.RuleCode, &o.LineID,
			&o.ImpactMinor, &o.Currency); err != nil {
			return d, wrap("scan open", err)
		}
		d.OpenTop = append(d.OpenTop, o)
	}
	return d, rows.Err()
}
