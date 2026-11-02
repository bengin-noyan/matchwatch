package ubl

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// ErrNotDespatch means the root element is not a UBL DespatchAdvice.
var ErrNotDespatch = errors.New("ubl: document is not a DespatchAdvice")

type DespatchLine struct {
	LineID, ItemCode, Barcode, Name, UOM string
	DeliveredQty                         decimal.Decimal
}

type DespatchAdvice struct {
	ETTN, Number       string
	IssueDate          time.Time
	ShipmentDate       *time.Time
	Supplier, Customer Party
	Lines              []DespatchLine
}

type xmlDespatch struct {
	UUID      string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 UUID"`
	ID        string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
	IssueDate string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 IssueDate"`
	Supplier  struct {
		Party xmlParty `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Party"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 DespatchSupplierParty"`
	Customer struct {
		Party xmlParty `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Party"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 DeliveryCustomerParty"`
	Shipment struct {
		Delivery struct {
			Despatch struct {
				Date string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ActualDespatchDate"`
			} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Despatch"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Delivery"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Shipment"`
	Lines []struct {
		ID  string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
		Qty struct {
			Unit  string `xml:"unitCode,attr"`
			Value string `xml:",chardata"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 DeliveredQuantity"`
		Item struct {
			Name string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 Name"`
			Code struct {
				ID string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
			} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 SellersItemIdentification"`
			Barcode struct {
				ID string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
			} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 StandardItemIdentification"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Item"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 DespatchLine"`
}

// ParseDespatch decodes and validates a UBL-TR DespatchAdvice. It never panics on bad input.
func ParseDespatch(r io.Reader) (*DespatchAdvice, error) {
	var x xmlDespatch
	if err := decodeRoot(r, "urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2", "DespatchAdvice", ErrNotDespatch, &x); err != nil {
		return nil, err
	}
	d := &DespatchAdvice{ETTN: strings.TrimSpace(x.UUID), Number: strings.TrimSpace(x.ID)}
	u, err := uuid.Parse(d.ETTN)
	if err != nil {
		return nil, &ValidationError{"UUID", "must be a valid UUID"}
	}
	d.ETTN = u.String()
	if d.Number == "" {
		return nil, &ValidationError{"ID", "must not be empty"}
	}
	if d.IssueDate, err = parseDate("IssueDate", x.IssueDate); err != nil {
		return nil, err
	}
	if d.ShipmentDate, err = parseOptDate("ActualDespatchDate", x.Shipment.Delivery.Despatch.Date); err != nil {
		return nil, err
	}
	if d.Supplier, err = x.Supplier.Party.party("Supplier"); err != nil {
		return nil, err
	}
	if d.Customer, err = x.Customer.Party.party("Customer"); err != nil {
		return nil, err
	}
	if len(x.Lines) == 0 {
		return nil, &ValidationError{"DespatchLine", "at least one line required"}
	}
	for _, l := range x.Lines {
		line := DespatchLine{
			LineID:   strings.TrimSpace(l.ID),
			ItemCode: strings.TrimSpace(l.Item.Code.ID),
			Barcode:  strings.TrimSpace(l.Item.Barcode.ID),
			Name:     strings.TrimSpace(l.Item.Name),
			UOM:      domain.NormalizeUOM(l.Qty.Unit),
		}
		if line.DeliveredQty, err = parseDecimal("DeliveredQuantity", l.Qty.Value); err != nil {
			return nil, err
		}
		if line.DeliveredQty.IsNegative() {
			return nil, &ValidationError{"DeliveredQuantity", "line " + line.LineID + ": must be >= 0"}
		}
		d.Lines = append(d.Lines, line)
	}
	return d, nil
}
