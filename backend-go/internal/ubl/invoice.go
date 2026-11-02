package ubl

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

const (
	nsInvoice = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	taxKDV    = "0015"
)

// ErrNotInvoice means the root element is not a UBL Invoice.
var ErrNotInvoice = errors.New("ubl: document is not an Invoice")

type DocRef struct {
	Number    string
	IssueDate *time.Time
}

type InvoiceLine struct {
	LineID, ItemCode, Barcode, Name, UOM              string
	Qty, UnitPrice, Allowance, LineExtension, VATRate decimal.Decimal
	WithholdingPercent, WithholdingAmount             decimal.Decimal
	WithholdingCode                                   string
}

type Invoice struct {
	ETTN, Number, ProfileID, Currency string
	IssueDate                         time.Time
	DueDate                           *time.Time
	ExchangeRate                      *domain.ExchangeRate
	Supplier, Customer                Party
	DespatchRefs                      []DocRef
	TaxExclusive, TaxAmount, Payable  decimal.Decimal
	Lines                             []InvoiceLine
}

type xmlTaxSubtotal struct {
	Percent  string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 Percent"`
	Category struct {
		Scheme struct {
			Code string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 TaxTypeCode"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 TaxScheme"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 TaxCategory"`
}

type xmlTaxTotal struct {
	Amount    string           `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 TaxAmount"`
	Subtotals []xmlTaxSubtotal `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 TaxSubtotal"`
}

type xmlInvoice struct {
	UUID      string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 UUID"`
	ID        string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
	ProfileID string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ProfileID"`
	IssueDate string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 IssueDate"`
	Currency  string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 DocumentCurrencyCode"`
	FX        *struct {
		Source string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 SourceCurrencyCode"`
		Target string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 TargetCurrencyCode"`
		Rate   string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 CalculationRate"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 PricingExchangeRate"`
	Despatch []struct {
		ID   string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
		Date string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 IssueDate"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 DespatchDocumentReference"`
	Supplier struct {
		Party xmlParty `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Party"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 AccountingSupplierParty"`
	Customer struct {
		Party xmlParty `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Party"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 AccountingCustomerParty"`
	Payment struct {
		DueDate string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 PaymentDueDate"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 PaymentMeans"`
	TaxTotal []xmlTaxTotal `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 TaxTotal"`
	Totals   struct {
		TaxExclusive string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 TaxExclusiveAmount"`
		Payable      string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 PayableAmount"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 LegalMonetaryTotal"`
	Lines []struct {
		ID  string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
		Qty struct {
			Unit  string `xml:"unitCode,attr"`
			Value string `xml:",chardata"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 InvoicedQuantity"`
		Ext        string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 LineExtensionAmount"`
		Allowances []struct {
			Charge string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ChargeIndicator"`
			Amount string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 Amount"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 AllowanceCharge"`
		Tax         []xmlTaxTotal `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 TaxTotal"`
		Withholding []xmlTaxTotal `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 WithholdingTaxTotal"`
		Item        struct {
			Name string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 Name"`
			Code struct {
				ID string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
			} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 SellersItemIdentification"`
			Barcode struct {
				ID string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
			} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 StandardItemIdentification"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Item"`
		Price struct {
			Amount string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 PriceAmount"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Price"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 InvoiceLine"`
}

// ParseInvoice decodes and validates a UBL-TR Invoice. It never panics on bad input.
func ParseInvoice(r io.Reader) (*Invoice, error) {
	dec := xml.NewDecoder(r)
	var start xml.StartElement
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if s, ok := tok.(xml.StartElement); ok {
			start = s
			break
		}
	}
	if start.Name.Space != nsInvoice || start.Name.Local != "Invoice" {
		return nil, ErrNotInvoice
	}
	var x xmlInvoice
	if err := dec.DecodeElement(&x, &start); err != nil {
		return nil, err
	}
	return x.convert()
}

func (x *xmlInvoice) convert() (*Invoice, error) {
	inv := &Invoice{
		ETTN:      strings.TrimSpace(x.UUID),
		Number:    strings.TrimSpace(x.ID),
		ProfileID: strings.TrimSpace(x.ProfileID),
		Currency:  strings.TrimSpace(x.Currency),
	}
	u, err := uuid.Parse(inv.ETTN)
	if err != nil {
		return nil, &ValidationError{"UUID", "must be a valid UUID"}
	}
	inv.ETTN = u.String()
	if inv.Number == "" {
		return nil, &ValidationError{"ID", "must not be empty"}
	}
	if inv.IssueDate, err = parseDate("IssueDate", x.IssueDate); err != nil {
		return nil, err
	}
	if inv.DueDate, err = parseOptDate("PaymentDueDate", x.Payment.DueDate); err != nil {
		return nil, err
	}
	if x.FX != nil {
		rate, err := parseDecimal("CalculationRate", x.FX.Rate)
		if err != nil {
			return nil, err
		}
		inv.ExchangeRate = &domain.ExchangeRate{
			Source: strings.TrimSpace(x.FX.Source), Target: strings.TrimSpace(x.FX.Target), Rate: rate,
		}
	}
	for _, d := range x.Despatch {
		t, err := parseOptDate("DespatchDocumentReference.IssueDate", d.Date)
		if err != nil {
			return nil, err
		}
		inv.DespatchRefs = append(inv.DespatchRefs, DocRef{Number: strings.TrimSpace(d.ID), IssueDate: t})
	}
	if inv.Supplier, err = x.Supplier.Party.party("Supplier"); err != nil {
		return nil, err
	}
	if inv.Customer, err = x.Customer.Party.party("Customer"); err != nil {
		return nil, err
	}
	if inv.TaxExclusive, err = parseDecimal("TaxExclusiveAmount", x.Totals.TaxExclusive); err != nil {
		return nil, err
	}
	for _, t := range x.TaxTotal {
		amt, err := parseDecimal("TaxAmount", t.Amount)
		if err != nil {
			return nil, err
		}
		inv.TaxAmount = inv.TaxAmount.Add(amt)
	}
	if inv.Payable, err = parseDecimal("PayableAmount", x.Totals.Payable); err != nil {
		return nil, err
	}
	if len(x.Lines) == 0 {
		return nil, &ValidationError{"InvoiceLine", "at least one line required"}
	}
	for _, l := range x.Lines {
		line := InvoiceLine{
			LineID:   strings.TrimSpace(l.ID),
			ItemCode: strings.TrimSpace(l.Item.Code.ID),
			Barcode:  strings.TrimSpace(l.Item.Barcode.ID),
			Name:     strings.TrimSpace(l.Item.Name),
			UOM:      domain.NormalizeUOM(l.Qty.Unit),
		}
		if line.Qty, err = parseDecimal("InvoicedQuantity", l.Qty.Value); err != nil {
			return nil, err
		}
		if !line.Qty.IsPositive() {
			return nil, &ValidationError{"InvoicedQuantity", "line " + line.LineID + ": must be > 0"}
		}
		if line.LineExtension, err = parseDecimal("LineExtensionAmount", l.Ext); err != nil {
			return nil, err
		}
		if line.LineExtension.IsNegative() {
			return nil, &ValidationError{"LineExtensionAmount", "line " + line.LineID + ": must be >= 0"}
		}
		if line.UnitPrice, err = parseDecimal("PriceAmount", l.Price.Amount); err != nil {
			return nil, err
		}
		for _, a := range l.Allowances {
			if !strings.EqualFold(strings.TrimSpace(a.Charge), "false") {
				continue
			}
			amt, err := parseDecimal("AllowanceCharge.Amount", a.Amount)
			if err != nil {
				return nil, err
			}
			line.Allowance = line.Allowance.Add(amt)
		}
	vat:
		for _, t := range l.Tax {
			for _, s := range t.Subtotals {
				if strings.TrimSpace(s.Category.Scheme.Code) == taxKDV {
					if line.VATRate, err = parseDecimal("Percent", s.Percent); err != nil {
						return nil, err
					}
					break vat
				}
			}
		}
		var wh []xmlTaxSubtotal
		for _, t := range l.Withholding {
			amt, err := parseDecimal("WithholdingTaxTotal.TaxAmount", t.Amount)
			if err != nil {
				return nil, err
			}
			line.WithholdingAmount = line.WithholdingAmount.Add(amt)
			wh = append(wh, t.Subtotals...)
		}
		if len(wh) > 1 {
			return nil, &ValidationError{"WithholdingTaxTotal", "line " + line.LineID + ": more than one withholding subtotal"}
		}
		if len(wh) == 1 {
			line.WithholdingCode = strings.TrimSpace(wh[0].Category.Scheme.Code)
			if line.WithholdingPercent, err = parseDecimal("Percent", wh[0].Percent); err != nil {
				return nil, err
			}
		}
		inv.Lines = append(inv.Lines, line)
	}
	return inv, nil
}
