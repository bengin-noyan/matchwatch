package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type VendorRow struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	VKN          string          `json:"vkn"`
	ContractNo   string          `json:"contract_no"` // en yeni ACTIVE sözleşme, yoksa ""
	InvoiceCount int             `json:"invoice_count"`
	OpenFindings int             `json:"open_findings"`
	Caught       json.RawMessage `json:"caught"` // [{currency, caught_minor}], APPROVED_EXCEPTION dışındaki bulgular
}

const vendorCols = `v.id, v.name, v.vkn,
	COALESCE((SELECT c.contract_no FROM contracts c WHERE c.tenant_id=v.tenant_id AND c.vendor_id=v.id AND c.status='ACTIVE'
		ORDER BY c.valid_from DESC, c.id LIMIT 1), ''),
	(SELECT count(*) FROM invoices i WHERE i.tenant_id=v.tenant_id AND i.vendor_id=v.id),
	(SELECT count(*) FROM audit_findings f JOIN invoices i ON i.tenant_id=f.tenant_id AND i.id=f.invoice_id
		WHERE f.tenant_id=v.tenant_id AND i.vendor_id=v.id AND f.resolution_status='OPEN'),
	COALESCE((SELECT json_agg(json_build_object('currency', cur, 'caught_minor', amt) ORDER BY amt DESC) FROM (
		SELECT COALESCE(f.impact_currency, i.currency) AS cur, SUM(f.financial_impact_minor)::bigint AS amt
		FROM audit_findings f JOIN invoices i ON i.tenant_id=f.tenant_id AND i.id=f.invoice_id
		WHERE f.tenant_id=v.tenant_id AND i.vendor_id=v.id AND f.resolution_status <> 'APPROVED_EXCEPTION' GROUP BY 1) t), '[]')`

func scanVendor(row pgx.Row, v *VendorRow) error {
	return row.Scan(&v.ID, &v.Name, &v.VKN, &v.ContractNo, &v.InvoiceCount, &v.OpenFindings, &v.Caught)
}

func (tx *Tx) ListVendors(ctx context.Context) ([]VendorRow, error) {
	rows, err := tx.Query(ctx, `SELECT `+vendorCols+` FROM vendors v WHERE v.tenant_id=$1 ORDER BY v.name, v.id`, tx.tenantID)
	if err != nil {
		return nil, wrap("list vendors", err)
	}
	defer rows.Close()
	out := []VendorRow{}
	for rows.Next() {
		var v VendorRow
		if err := scanVendor(rows, &v); err != nil {
			return nil, wrap("scan vendor", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type ContractRow struct {
	ID              uuid.UUID         `json:"id"`
	ContractNo      string            `json:"contract_no"`
	Status          string            `json:"status"`
	ValidFrom       string            `json:"valid_from"`
	ValidUntil      string            `json:"valid_until"`
	PaymentTermDays int               `json:"payment_term_days"`
	Currency        string            `json:"currency"`
	QtyTolerancePct string            `json:"qty_tolerance_pct"`
	Items           []ContractItemRow `json:"items"`
}

type PricePointRow struct {
	InvoiceID    uuid.UUID `json:"invoice_id"`
	InvoiceNo    string    `json:"invoice_no"`
	IssueDate    string    `json:"issue_date"`
	NetUnitPrice string    `json:"net_unit_price"` // line_extension / qty, SQL'de NUMERIC olarak (D2)
	Currency     string    `json:"currency"`
	UOM          string    `json:"uom"`
}

type VendorDetail struct {
	VendorRow
	Contracts    []ContractRow              `json:"contracts"`
	PriceHistory map[string][]PricePointRow `json:"price_history"` // sözleşme kalemi id'sine göre, eskiden yeniye
}

func (tx *Tx) VendorDetail(ctx context.Context, id uuid.UUID) (VendorDetail, error) {
	d := VendorDetail{Contracts: []ContractRow{}, PriceHistory: map[string][]PricePointRow{}}
	err := scanVendor(tx.QueryRow(ctx, `SELECT `+vendorCols+` FROM vendors v WHERE v.tenant_id=$1 AND v.id=$2`, tx.tenantID, id), &d.VendorRow)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, wrap("vendor", err)
	}
	rows, err := tx.Query(ctx, `SELECT id, contract_no, status, valid_from::text, valid_until::text, payment_term_days, currency,
		qty_tolerance_pct::text FROM contracts WHERE tenant_id=$1 AND vendor_id=$2 ORDER BY valid_from DESC, id`, tx.tenantID, id)
	if err != nil {
		return d, wrap("vendor contracts", err)
	}
	for rows.Next() {
		c := ContractRow{Items: []ContractItemRow{}}
		if err := rows.Scan(&c.ID, &c.ContractNo, &c.Status, &c.ValidFrom, &c.ValidUntil, &c.PaymentTermDays, &c.Currency, &c.QtyTolerancePct); err != nil {
			rows.Close()
			return d, wrap("scan contract", err)
		}
		d.Contracts = append(d.Contracts, c)
	}
	rows.Close()
	for i := range d.Contracts {
		c := &d.Contracts[i]
		irows, err := tx.Query(ctx, `SELECT id, item_code, item_name, unit_price::text, currency, uom, source_page, source_clause_text
			FROM contract_items WHERE tenant_id=$1 AND contract_id=$2 ORDER BY item_name, id`, tx.tenantID, c.ID)
		if err != nil {
			return d, wrap("contract items", err)
		}
		for irows.Next() {
			it := ContractItemRow{ContractNo: c.ContractNo}
			if err := irows.Scan(&it.ID, &it.ItemCode, &it.Name, &it.UnitPrice, &it.Currency, &it.UOM, &it.SourcePage, &it.SourceClauseText); err != nil {
				irows.Close()
				return d, wrap("scan contract item", err)
			}
			c.Items = append(c.Items, it)
		}
		irows.Close()
	}
	prows, err := tx.Query(ctx, `SELECT l.matched_contract_item_id, i.id, i.invoice_no, i.issue_date::text,
		round(l.line_extension_minor::numeric / 100 / l.qty, 6)::text, i.currency, l.uom
		FROM invoice_lines l JOIN invoices i ON i.tenant_id=l.tenant_id AND i.id=l.invoice_id
		WHERE l.tenant_id=$1 AND i.vendor_id=$2 AND l.matched_contract_item_id IS NOT NULL
		ORDER BY i.issue_date, i.created_at, l.id`, tx.tenantID, id)
	if err != nil {
		return d, wrap("price history", err)
	}
	defer prows.Close()
	for prows.Next() {
		var item uuid.UUID
		var p PricePointRow
		if err := prows.Scan(&item, &p.InvoiceID, &p.InvoiceNo, &p.IssueDate, &p.NetUnitPrice, &p.Currency, &p.UOM); err != nil {
			return d, wrap("scan price point", err)
		}
		d.PriceHistory[item.String()] = append(d.PriceHistory[item.String()], p)
	}
	return d, prows.Err()
}
