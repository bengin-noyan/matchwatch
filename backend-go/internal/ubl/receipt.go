package ubl

import (
	"errors"
	"io"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrNotReceipt means the root element is not a UBL ReceiptAdvice.
var ErrNotReceipt = errors.New("ubl: document is not a ReceiptAdvice")

type ReceiptLine struct {
	DespatchLineID string
	RejectedQty    decimal.Decimal
}

type ReceiptAdvice struct {
	DespatchNumber string
	Lines          []ReceiptLine
}

type xmlReceipt struct {
	Despatch struct {
		ID string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 DespatchDocumentReference"`
	Lines []struct {
		Rejected string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 RejectedQuantity"`
		Ref      struct {
			LineID string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 LineID"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 DespatchLineReference"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 ReceiptLine"`
}

// ParseReceipt decodes and validates a UBL-TR ReceiptAdvice (Irsaliye Yaniti). It never panics on bad input.
func ParseReceipt(r io.Reader) (*ReceiptAdvice, error) {
	var x xmlReceipt
	if err := decodeRoot(r, "urn:oasis:names:specification:ubl:schema:xsd:ReceiptAdvice-2", "ReceiptAdvice", ErrNotReceipt, &x); err != nil {
		return nil, err
	}
	out := &ReceiptAdvice{DespatchNumber: strings.TrimSpace(x.Despatch.ID)}
	if out.DespatchNumber == "" {
		return nil, &ValidationError{"DespatchDocumentReference.ID", "must not be empty"}
	}
	if len(x.Lines) == 0 {
		return nil, &ValidationError{"ReceiptLine", "at least one line required"}
	}
	for _, l := range x.Lines {
		line := ReceiptLine{DespatchLineID: strings.TrimSpace(l.Ref.LineID)}
		if line.DespatchLineID == "" {
			return nil, &ValidationError{"DespatchLineReference.LineID", "must not be empty"}
		}
		var err error
		if line.RejectedQty, err = parseDecimal("RejectedQuantity", l.Rejected); err != nil {
			return nil, err
		}
		if line.RejectedQty.IsNegative() {
			return nil, &ValidationError{"RejectedQuantity", "line " + line.DespatchLineID + ": must be >= 0"}
		}
		out.Lines = append(out.Lines, line)
	}
	return out, nil
}
