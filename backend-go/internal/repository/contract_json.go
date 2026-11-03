package repository

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/shopspring/decimal"
)

// ContractDoc is a decoded contract JSON document; the vendor still has to be upserted by the caller.
type ContractDoc struct {
	VendorVKN, VendorName string
	ContractInput
}

type contractJSON struct {
	VendorVKN       string `json:"vendor_vkn"`
	VendorName      string `json:"vendor_name"`
	ContractNo      string `json:"contract_no"`
	ValidFrom       string `json:"valid_from"`
	ValidUntil      string `json:"valid_until"`
	PaymentTermDays int    `json:"payment_term_days"`
	Currency        string `json:"currency"`
	QtyTolerancePct string `json:"qty_tolerance_pct"`
	Items           []struct {
		ItemCode         string `json:"item_code"`
		Barcode          string `json:"barcode"`
		Name             string `json:"name"`
		UnitPrice        string `json:"unit_price"`
		Currency         string `json:"currency"`
		UOM              string `json:"uom"`
		WithholdingCode  string `json:"withholding_code"`
		RequiresDelivery *bool  `json:"requires_delivery"` // absent means true
		SourcePage       int    `json:"source_page"`
		SourceClauseText string `json:"source_clause_text"`
	} `json:"items"`
}

// ParseContractJSON decodes the contract import format (decimals as strings, dates as YYYY-MM-DD).
func ParseContractJSON(r io.Reader) (ContractDoc, error) {
	var j contractJSON
	if err := json.NewDecoder(r).Decode(&j); err != nil {
		return ContractDoc{}, fmt.Errorf("contract json: %w", err)
	}
	if j.VendorVKN == "" || j.ContractNo == "" || len(j.Items) == 0 {
		return ContractDoc{}, fmt.Errorf("contract json: vendor_vkn, contract_no and items are required")
	}
	doc := ContractDoc{VendorVKN: j.VendorVKN, VendorName: j.VendorName}
	c := &doc.ContractInput
	c.Number, c.PaymentTermDays, c.Currency = j.ContractNo, j.PaymentTermDays, j.Currency
	var err error
	if c.ValidFrom, err = time.Parse("2006-01-02", j.ValidFrom); err != nil {
		return ContractDoc{}, fmt.Errorf("contract json: valid_from: %w", err)
	}
	if c.ValidUntil, err = time.Parse("2006-01-02", j.ValidUntil); err != nil {
		return ContractDoc{}, fmt.Errorf("contract json: valid_until: %w", err)
	}
	if c.QtyTolerancePct, err = decimal.NewFromString(j.QtyTolerancePct); err != nil {
		return ContractDoc{}, fmt.Errorf("contract json: qty_tolerance_pct: %w", err)
	}
	for _, it := range j.Items {
		price, err := decimal.NewFromString(it.UnitPrice)
		if err != nil {
			return ContractDoc{}, fmt.Errorf("contract json: item %q unit_price: %w", it.ItemCode, err)
		}
		c.Items = append(c.Items, ContractItemInput{
			ItemCode: it.ItemCode, Barcode: it.Barcode, Name: it.Name, UOM: it.UOM, Currency: it.Currency,
			WithholdingCode: it.WithholdingCode, SourceClauseText: it.SourceClauseText, UnitPrice: price,
			SourcePage: it.SourcePage, RequiresDelivery: it.RequiresDelivery == nil || *it.RequiresDelivery,
		})
	}
	return doc, nil
}
