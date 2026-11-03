package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// dec parses a numeric that was selected ::text, so no float is ever involved.
func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func (tx *Tx) LoadInvoice(ctx context.Context, id uuid.UUID) (domain.Invoice, error) {
	inv := domain.Invoice{ID: id}
	var fxSrc, fxTgt, fxRate *string
	err := tx.QueryRow(ctx, `SELECT vendor_id, invoice_no, currency, issue_date, due_date, fx_source, fx_target, fx_rate::text,
		payable_minor, EXISTS (SELECT 1 FROM invoice_despatch_refs r WHERE r.tenant_id=i.tenant_id AND r.invoice_id=i.id)
		FROM invoices i WHERE tenant_id=$1 AND id=$2`, tx.tenantID, id).
		Scan(&inv.VendorID, &inv.Number, &inv.Currency, &inv.IssueDate, &inv.DueDate, &fxSrc, &fxTgt, &fxRate,
			&inv.PayableMinor, &inv.HasDespatchRefs)
	if errors.Is(err, pgx.ErrNoRows) {
		return inv, ErrNotFound
	}
	if err != nil {
		return inv, wrap("load invoice", err)
	}
	if fxSrc != nil && fxTgt != nil && fxRate != nil {
		inv.ExchangeRate = &domain.ExchangeRate{Source: *fxSrc, Target: *fxTgt, Rate: dec(*fxRate)}
	}
	rows, err := tx.Query(ctx, `SELECT id, line_id, item_code, barcode, item_name, uom, qty::text, unit_price::text,
		allowance_amount_minor, line_extension_minor, vat_rate::text, COALESCE(withholding_code,''),
		COALESCE(withholding_rate,0)::text, withholding_amount_minor
		FROM invoice_lines WHERE tenant_id=$1 AND invoice_id=$2 ORDER BY length(line_id), line_id, id`, tx.tenantID, id)
	if err != nil {
		return inv, wrap("load invoice lines", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l domain.InvoiceLine
		var qty, price, vat, whPct string
		var allowance, ext, whAmt int64
		if err := rows.Scan(&l.ID, &l.LineID, &l.ItemCode, &l.Barcode, &l.Name, &l.UOM, &qty, &price, &allowance, &ext,
			&vat, &l.WithholdingCode, &whPct, &whAmt); err != nil {
			return inv, wrap("scan invoice line", err)
		}
		l.Qty, l.UnitPrice, l.VATRate, l.WithholdingPercent = dec(qty), dec(price), dec(vat), dec(whPct)
		l.AllowanceAmount, l.LineExtension, l.WithholdingAmount = domain.FromMinor(allowance), domain.FromMinor(ext), domain.FromMinor(whAmt)
		inv.Lines = append(inv.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return inv, err
	}
	return inv, nil
}

// ActiveContract returns the ACTIVE contract covering `on` (latest valid_from wins), or nil when none.
func (tx *Tx) ActiveContract(ctx context.Context, vendorID uuid.UUID, on time.Time) (*domain.Contract, []domain.ContractItem, error) {
	var c domain.Contract
	var tol string
	err := tx.QueryRow(ctx, `SELECT id, contract_no, payment_term_days, currency, qty_tolerance_pct::text FROM contracts
		WHERE tenant_id=$1 AND vendor_id=$2 AND status='ACTIVE' AND valid_from <= $3 AND valid_until >= $3
		ORDER BY valid_from DESC, id LIMIT 1`, tx.tenantID, vendorID, on).
		Scan(&c.ID, &c.Number, &c.PaymentTermDays, &c.Currency, &tol)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, wrap("active contract", err)
	}
	c.QtyTolerancePct = dec(tol)
	rows, err := tx.Query(ctx, `SELECT id, item_code, barcode, item_name, uom, currency, COALESCE(withholding_code,''),
		source_clause_text, unit_price::text, source_page, requires_delivery
		FROM contract_items WHERE tenant_id=$1 AND contract_id=$2 ORDER BY id`, tx.tenantID, c.ID)
	if err != nil {
		return nil, nil, wrap("contract items", err)
	}
	defer rows.Close()
	var items []domain.ContractItem
	for rows.Next() {
		var it domain.ContractItem
		var price string
		if err := rows.Scan(&it.ID, &it.ItemCode, &it.Barcode, &it.Name, &it.UOM, &it.Currency, &it.WithholdingCode,
			&it.SourceClauseText, &price, &it.SourcePage, &it.RequiresDelivery); err != nil {
			return nil, nil, wrap("scan contract item", err)
		}
		it.UnitPrice = dec(price)
		items = append(items, it)
	}
	return &c, items, rows.Err()
}

// DespatchCandidates lists despatch lines the invoice may draw on, locking them so concurrent audits serialize.
// Available = delivered - rejected - allocations made by other invoices.
func (tx *Tx) DespatchCandidates(ctx context.Context, inv domain.Invoice) ([]domain.DespatchCandidate, error) {
	const from = ` FROM despatch_lines dl JOIN despatch_advices da ON da.tenant_id=dl.tenant_id AND da.id=dl.despatch_id
		WHERE dl.tenant_id=$1 AND da.vendor_id=$2 AND `
	where, args := `da.despatch_no IN (SELECT despatch_no FROM invoice_despatch_refs WHERE tenant_id=$1 AND invoice_id=$3)`,
		[]any{tx.tenantID, inv.VendorID, inv.ID}
	if !inv.HasDespatchRefs {
		where, args = `da.issue_date BETWEEN $4::date - 30 AND $4::date`, append(args, inv.IssueDate)
	}
	// Lock first, in its own statement: the Available query below must take a fresh READ COMMITTED snapshot
	// after any competing audit has committed its allocations (the locked rows themselves are never updated).
	if _, err := tx.Exec(ctx, `SELECT dl.id`+from+where+` ORDER BY dl.id FOR UPDATE OF dl`, args...); err != nil {
		return nil, wrap("lock despatch lines", err)
	}
	rows, err := tx.Query(ctx, `SELECT dl.id, dl.item_code, dl.barcode, dl.uom,
		(dl.delivered_qty - dl.rejected_qty - COALESCE((SELECT SUM(a.qty) FROM despatch_allocations a
			JOIN invoice_lines il ON il.tenant_id=a.tenant_id AND il.id=a.invoice_line_id
			WHERE a.tenant_id=dl.tenant_id AND a.despatch_line_id=dl.id AND il.invoice_id <> $3), 0))::text,
		da.shipment_date`+from+where+` ORDER BY dl.id`, args...)
	if err != nil {
		return nil, wrap("despatch candidates", err)
	}
	defer rows.Close()
	var out []domain.DespatchCandidate
	for rows.Next() {
		var c domain.DespatchCandidate
		var avail string
		if err := rows.Scan(&c.LineID, &c.ItemCode, &c.Barcode, &c.UOM, &avail, &c.ShipmentDate); err != nil {
			return nil, wrap("scan despatch candidate", err)
		}
		c.Available = dec(avail)
		out = append(out, c)
	}
	return out, rows.Err()
}

// PriceHistory returns the vendor's earlier net unit prices for the same item, oldest first,
// excluding the invoice being audited. shortcut: unbounded history, cap it if vendors accumulate thousands of lines.
func (tx *Tx) PriceHistory(ctx context.Context, inv domain.Invoice, line domain.InvoiceLine) ([]domain.PricePoint, error) {
	if line.ItemCode == "" && line.Barcode == "" {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT i.issue_date, l.line_extension_minor, l.qty::text
		FROM invoice_lines l JOIN invoices i ON i.tenant_id=l.tenant_id AND i.id=l.invoice_id
		WHERE l.tenant_id=$1 AND i.vendor_id=$2 AND i.id <> $3 AND i.issue_date <= $4 AND l.qty > 0
		  AND ((l.item_code <> '' AND l.item_code=$5) OR (l.barcode <> '' AND l.barcode=$6))
		ORDER BY i.issue_date, i.created_at, i.id, l.id`, tx.tenantID, inv.VendorID, inv.ID, inv.IssueDate, line.ItemCode, line.Barcode)
	if err != nil {
		return nil, wrap("price history", err)
	}
	defer rows.Close()
	var out []domain.PricePoint
	for rows.Next() {
		var p domain.PricePoint
		var ext int64
		var qty string
		if err := rows.Scan(&p.IssueDate, &ext, &qty); err != nil {
			return nil, wrap("scan price point", err)
		}
		p.NetUnitPrice = domain.FromMinor(ext).Div(dec(qty))
		out = append(out, p)
	}
	return out, rows.Err()
}

// DuplicateCandidates lists the vendor's other invoices issued within 90 days of inv.
func (tx *Tx) DuplicateCandidates(ctx context.Context, inv domain.Invoice) ([]domain.DuplicateCandidate, error) {
	rows, err := tx.Query(ctx, `SELECT i.id, i.invoice_no, i.issue_date, i.payable_minor,
		COALESCE(array_agg(l.item_name ORDER BY l.id) FILTER (WHERE l.id IS NOT NULL), '{}')
		FROM invoices i LEFT JOIN invoice_lines l ON l.tenant_id=i.tenant_id AND l.invoice_id=i.id
		WHERE i.tenant_id=$1 AND i.vendor_id=$2 AND i.id <> $3 AND abs(i.issue_date - $4::date) <= 90
		GROUP BY i.id ORDER BY i.issue_date, i.id`, tx.tenantID, inv.VendorID, inv.ID, inv.IssueDate)
	if err != nil {
		return nil, wrap("duplicate candidates", err)
	}
	defer rows.Close()
	var out []domain.DuplicateCandidate
	for rows.Next() {
		var c domain.DuplicateCandidate
		if err := rows.Scan(&c.InvoiceID, &c.Number, &c.IssueDate, &c.PayableMinor, &c.LineNames); err != nil {
			return nil, wrap("scan duplicate candidate", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// WithholdingPercents maps code -> percent; codes with a NULL percent are skipped.
func (tx *Tx) WithholdingPercents(ctx context.Context) (map[string]decimal.Decimal, error) {
	rows, err := tx.Query(ctx, `SELECT code, percent::text FROM withholding_codes WHERE percent IS NOT NULL`)
	if err != nil {
		return nil, wrap("withholding codes", err)
	}
	defer rows.Close()
	out := map[string]decimal.Decimal{}
	for rows.Next() {
		var code, pct string
		if err := rows.Scan(&code, &pct); err != nil {
			return nil, wrap("scan withholding code", err)
		}
		out[code] = dec(pct)
	}
	return out, rows.Err()
}

func (tx *Tx) FinancingRate(ctx context.Context) (decimal.Decimal, error) {
	var r string
	err := tx.QueryRow(ctx, `SELECT financing_rate_annual::text FROM tenants WHERE id=$1`, tx.tenantID).Scan(&r)
	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, ErrNotFound
	}
	if err != nil {
		return decimal.Zero, wrap("financing rate", err)
	}
	return dec(r), nil
}
