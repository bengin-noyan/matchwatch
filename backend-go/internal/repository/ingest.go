package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/ubl"
)

type ContractInput struct {
	VendorID              uuid.UUID
	Number                string
	ValidFrom, ValidUntil time.Time
	PaymentTermDays       int
	Currency              string
	QtyTolerancePct       decimal.Decimal
	FilePath              string
	Items                 []ContractItemInput
}

type ContractItemInput struct {
	ItemCode, Barcode, Name, UOM, Currency, WithholdingCode, SourceClauseText string
	UnitPrice                                                                 decimal.Decimal
	SourcePage                                                                int
	RequiresDelivery                                                          bool
}

func (tx *Tx) TenantVKN(ctx context.Context) (string, error) {
	var vkn string
	err := tx.QueryRow(ctx, `SELECT vkn FROM tenants WHERE id=$1`, tx.tenantID).Scan(&vkn)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return vkn, err
}

func (tx *Tx) UpsertVendor(ctx context.Context, vkn, title string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO vendors(tenant_id, vkn, name) VALUES ($1,$2,$3)
		ON CONFLICT (tenant_id, vkn) DO UPDATE SET name = EXCLUDED.name RETURNING id`,
		tx.tenantID, vkn, title).Scan(&id)
	return id, wrap("upsert vendor", err)
}

func (tx *Tx) InsertContract(ctx context.Context, c ContractInput) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO contracts(tenant_id, vendor_id, contract_no, valid_from, valid_until,
		payment_term_days, currency, qty_tolerance_pct, file_path) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		tx.tenantID, c.VendorID, c.Number, c.ValidFrom, c.ValidUntil, c.PaymentTermDays, c.Currency,
		num(c.QtyTolerancePct), c.FilePath).Scan(&id)
	if isUniqueOn(err, "contract_no") {
		return id, ErrDuplicateContract
	}
	if err != nil {
		return id, wrap("insert contract", err)
	}
	for _, it := range c.Items {
		var wh *string
		if it.WithholdingCode != "" {
			wh = &it.WithholdingCode
		}
		_, err := tx.Exec(ctx, `INSERT INTO contract_items(tenant_id, contract_id, item_code, barcode, item_name,
			unit_price, currency, uom, withholding_code, requires_delivery, source_clause_text, source_page)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			tx.tenantID, id, it.ItemCode, it.Barcode, it.Name, num(it.UnitPrice), it.Currency,
			domain.NormalizeUOM(it.UOM), wh, it.RequiresDelivery, it.SourceClauseText, it.SourcePage)
		if err != nil {
			return id, wrap("insert contract item", err)
		}
	}
	return id, nil
}

// InsertDespatch stores the despatch and its lines.
func (tx *Tx) InsertDespatch(ctx context.Context, vendorID uuid.UUID, d *ubl.DespatchAdvice, rawPath string) (uuid.UUID, error) {
	ship := d.IssueDate // shipment_date is NOT NULL; fall back to the issue date
	if d.ShipmentDate != nil {
		ship = *d.ShipmentDate
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO despatch_advices(tenant_id, vendor_id, ettn, despatch_no, issue_date, shipment_date, raw_xml_path)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, tx.tenantID, vendorID, d.ETTN, d.Number, d.IssueDate, ship, rawPath).Scan(&id)
	if isUniqueOn(err, "ettn") {
		return id, ErrDuplicateETTN
	}
	if err != nil {
		return id, wrap("insert despatch", err)
	}
	for _, l := range d.Lines {
		_, err := tx.Exec(ctx, `INSERT INTO despatch_lines(tenant_id, despatch_id, line_id, item_code, barcode, item_name, uom, delivered_qty)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, tx.tenantID, id, l.LineID, l.ItemCode, l.Barcode, l.Name,
			domain.NormalizeUOM(l.UOM), num(l.DeliveredQty))
		if err != nil {
			return id, wrap("insert despatch line", err)
		}
	}
	return id, nil
}

// ApplyReceipt sets rejected_qty on the referenced despatch's lines.
func (tx *Tx) ApplyReceipt(ctx context.Context, vendorID uuid.UUID, r *ubl.ReceiptAdvice) (string, error) {
	var despatchID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM despatch_advices WHERE tenant_id=$1 AND vendor_id=$2 AND despatch_no=$3
		ORDER BY created_at DESC, id LIMIT 1`, tx.tenantID, vendorID, r.DespatchNumber).Scan(&despatchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", ErrUnknownDespatch, r.DespatchNumber)
	}
	if err != nil {
		return "", wrap("find despatch", err)
	}
	for _, l := range r.Lines {
		tag, err := tx.Exec(ctx, `UPDATE despatch_lines SET rejected_qty=$1 WHERE tenant_id=$2 AND despatch_id=$3 AND line_id=$4`,
			num(l.RejectedQty), tx.tenantID, despatchID, l.DespatchLineID)
		if err != nil {
			return "", wrap("apply receipt line "+l.DespatchLineID, err) // includes the rejected <= delivered CHECK
		}
		if tag.RowsAffected() == 0 {
			return "", fmt.Errorf("%w: %s has no line %s", ErrUnknownDespatch, r.DespatchNumber, l.DespatchLineID)
		}
	}
	return r.DespatchNumber, nil
}

// InsertInvoice stores the invoice, its lines and despatch references.
func (tx *Tx) InsertInvoice(ctx context.Context, vendorID uuid.UUID, inv *ubl.Invoice, rawPath string) (uuid.UUID, error) {
	var fxSrc, fxTgt *string
	var fxRate any
	if fx := inv.ExchangeRate; fx != nil {
		fxSrc, fxTgt, fxRate = &fx.Source, &fx.Target, num(fx.Rate)
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO invoices(tenant_id, vendor_id, ettn, invoice_no, issue_date, due_date, currency,
		fx_source, fx_target, fx_rate, payable_minor, raw_xml_path) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		tx.tenantID, vendorID, inv.ETTN, inv.Number, inv.IssueDate, inv.DueDate, inv.Currency,
		fxSrc, fxTgt, fxRate, domain.ToMinor(inv.Payable), rawPath).Scan(&id)
	if isUniqueOn(err, "ettn") {
		return id, ErrDuplicateETTN
	}
	if err != nil {
		return id, wrap("insert invoice", err)
	}
	for _, l := range inv.Lines {
		var wh *string
		var whRate any
		if l.WithholdingCode != "" {
			wh, whRate = &l.WithholdingCode, num(l.WithholdingPercent)
		}
		_, err := tx.Exec(ctx, `INSERT INTO invoice_lines(tenant_id, invoice_id, line_id, item_code, barcode, item_name, uom,
			qty, unit_price, allowance_amount_minor, line_extension_minor, vat_rate, withholding_code, withholding_rate,
			withholding_amount_minor) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			tx.tenantID, id, l.LineID, l.ItemCode, l.Barcode, l.Name, domain.NormalizeUOM(l.UOM),
			num(l.Qty), num(l.UnitPrice), domain.ToMinor(l.Allowance), domain.ToMinor(l.LineExtension),
			num(l.VATRate), wh, whRate, domain.ToMinor(l.WithholdingAmount))
		if err != nil {
			return id, wrap("insert invoice line "+l.LineID, err)
		}
	}
	for _, r := range inv.DespatchRefs {
		if _, err := tx.Exec(ctx, `INSERT INTO invoice_despatch_refs(tenant_id, invoice_id, despatch_no, issue_date) VALUES ($1,$2,$3,$4)`,
			tx.tenantID, id, r.Number, r.IssueDate); err != nil {
			return id, wrap("insert despatch ref", err)
		}
	}
	return id, nil
}

func (tx *Tx) InvoicesReferencingDespatch(ctx context.Context, vendorID uuid.UUID, despatchNo string) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT i.id FROM invoices i
		JOIN invoice_despatch_refs r ON r.tenant_id=i.tenant_id AND r.invoice_id=i.id
		WHERE i.tenant_id=$1 AND i.vendor_id=$2 AND r.despatch_no=$3`, tx.tenantID, vendorID, despatchNo)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
