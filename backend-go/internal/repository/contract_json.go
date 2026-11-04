package repository

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ContractDoc is a decoded contract JSON document; the vendor still has to be upserted by the caller.
type ContractDoc struct {
	VendorVKN, VendorName string
	DraftID               *uuid.UUID // set when approving a reviewed extraction draft
	ContractInput
}

var (
	vknRe      = regexp.MustCompile(`^(\d{10}|\d{11})$`)
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	maxPrice   = decimal.New(1, 13) // NUMERIC(19,6) holds 13 integer digits
)

// contractJSON uses pointers for fields that must be present; decimal.Decimal parses JSON strings and numbers
// from their text, so no float is involved.
type contractJSON struct {
	VendorVKN       string           `json:"vendor_vkn"`
	VendorName      string           `json:"vendor_name"`
	DraftID         *uuid.UUID       `json:"draft_id"`
	ContractNo      string           `json:"contract_no"`
	ValidFrom       string           `json:"valid_from"`
	ValidUntil      string           `json:"valid_until"`
	PaymentTermDays *int             `json:"payment_term_days"`
	Currency        string           `json:"currency"`
	QtyTolerancePct *decimal.Decimal `json:"qty_tolerance_pct"`
	PaymentTermPage *int             `json:"payment_term_source_page"`
	PaymentTermText string           `json:"payment_term_clause_text"`
	Items           []struct {
		ItemCode         string           `json:"item_code"`
		Barcode          string           `json:"barcode"`
		Name             string           `json:"name"`
		UnitPrice        *decimal.Decimal `json:"unit_price"`
		Currency         string           `json:"currency"`
		UOM              string           `json:"uom"`
		WithholdingCode  string           `json:"withholding_code"`
		RequiresDelivery *bool            `json:"requires_delivery"` // absent means true
		SourcePage       int              `json:"source_page"`
		SourceClauseText string           `json:"source_clause_text"`
	} `json:"items"`
}

// sane rejects absurd exponents before any arithmetic: Cmp on 1e999999999 would allocate gigabytes.
func sane(d decimal.Decimal) bool { return d.Exponent() >= -12 && d.Exponent() <= 12 }

// ParseContractJSON decodes and validates the contract import format (decimals as strings or numbers, dates as YYYY-MM-DD).
func ParseContractJSON(r io.Reader) (ContractDoc, error) {
	var j contractJSON
	if err := json.NewDecoder(r).Decode(&j); err != nil {
		return ContractDoc{}, fmt.Errorf("contract json: %w", err)
	}
	bad := func(format string, a ...any) (ContractDoc, error) {
		return ContractDoc{}, fmt.Errorf("contract json: "+format, a...)
	}
	switch {
	case !vknRe.MatchString(j.VendorVKN):
		return bad("vendor_vkn must be 10 or 11 digits")
	case j.ContractNo == "" || len(j.ContractNo) > 100:
		return bad("contract_no is required (max 100 chars)")
	case j.PaymentTermDays == nil || *j.PaymentTermDays < 0 || *j.PaymentTermDays > 3650:
		return bad("payment_term_days is required and must be 0..3650")
	case !currencyRe.MatchString(j.Currency):
		return bad("currency must be three uppercase letters")
	case j.QtyTolerancePct == nil || !sane(*j.QtyTolerancePct) || j.QtyTolerancePct.IsNegative() || j.QtyTolerancePct.GreaterThan(decimal.NewFromInt(100)):
		return bad("qty_tolerance_pct is required and must be 0..100")
	case j.PaymentTermPage != nil && (*j.PaymentTermPage < 1 || *j.PaymentTermPage > 100000):
		return bad("payment_term_source_page must be >= 1")
	case len(j.Items) == 0 || len(j.Items) > 5000:
		return bad("items must hold 1..5000 entries")
	}
	doc := ContractDoc{VendorVKN: j.VendorVKN, VendorName: j.VendorName, DraftID: j.DraftID}
	c := &doc.ContractInput
	c.Number, c.PaymentTermDays, c.Currency, c.QtyTolerancePct = j.ContractNo, *j.PaymentTermDays, j.Currency, *j.QtyTolerancePct
	if j.PaymentTermPage != nil {
		c.PaymentTermSourcePage = *j.PaymentTermPage
	}
	c.PaymentTermClauseText = j.PaymentTermText
	var err error
	if c.ValidFrom, err = time.Parse("2006-01-02", j.ValidFrom); err != nil {
		return bad("valid_from: %v", err)
	}
	if c.ValidUntil, err = time.Parse("2006-01-02", j.ValidUntil); err != nil {
		return bad("valid_until: %v", err)
	}
	if c.ValidFrom.After(c.ValidUntil) {
		return bad("valid_from must not be after valid_until")
	}
	for i, it := range j.Items {
		switch {
		case it.UnitPrice == nil || !sane(*it.UnitPrice) || it.UnitPrice.IsNegative() || it.UnitPrice.GreaterThanOrEqual(maxPrice):
			return bad("item %d: unit_price is required and must be >= 0", i)
		case !currencyRe.MatchString(it.Currency):
			return bad("item %d: currency must be three uppercase letters", i)
		case it.UOM == "":
			return bad("item %d: uom is required", i)
		case it.Name == "":
			return bad("item %d: name is required", i)
		case len(it.WithholdingCode) > 10:
			return bad("item %d: withholding_code too long", i)
		}
		c.Items = append(c.Items, ContractItemInput{
			ItemCode: it.ItemCode, Barcode: it.Barcode, Name: it.Name, UOM: it.UOM, Currency: it.Currency,
			WithholdingCode: it.WithholdingCode, SourceClauseText: it.SourceClauseText, UnitPrice: *it.UnitPrice,
			SourcePage: it.SourcePage, RequiresDelivery: it.RequiresDelivery == nil || *it.RequiresDelivery,
		})
	}
	return doc, nil
}
