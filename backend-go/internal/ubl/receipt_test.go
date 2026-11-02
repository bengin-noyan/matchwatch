package ubl

import (
	"errors"
	"strings"
	"testing"
)

func TestParseReceiptBasic(t *testing.T) {
	r, err := ParseReceipt(strings.NewReader(readFixture(t, "receipt_basic.xml")))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "DespatchNumber", r.DespatchNumber, "IRS2026000000001")
	if len(r.Lines) != 1 {
		t.Fatalf("Lines = %d", len(r.Lines))
	}
	eq(t, "DespatchLineID", r.Lines[0].DespatchLineID, "1")
	eq(t, "RejectedQty", r.Lines[0].RejectedQty.String(), "50")
}

func TestParseReceiptRejectsDespatch(t *testing.T) {
	if _, err := ParseReceipt(strings.NewReader(readFixture(t, "despatch_basic.xml"))); !errors.Is(err, ErrNotReceipt) {
		t.Fatalf("err = %v, want ErrNotReceipt", err)
	}
}

func TestParseReceiptEdgeCases(t *testing.T) {
	base := readFixture(t, "receipt_basic.xml")
	noRej := strings.Replace(base, `<cbc:RejectedQuantity unitCode="C62">50</cbc:RejectedQuantity>`, "", 1)
	r, err := ParseReceipt(strings.NewReader(noRej))
	if err != nil || !r.Lines[0].RejectedQty.IsZero() {
		t.Fatalf("missing rejected: %v %v", r, err)
	}
	var ve *ValidationError
	if _, err = ParseReceipt(strings.NewReader(strings.Replace(base, ">50<", ">-5<", 1))); !errors.As(err, &ve) {
		t.Fatalf("negative: err = %v", err)
	}
	blank := strings.Replace(base, "<cbc:LineID>1</cbc:LineID>", "<cbc:LineID> </cbc:LineID>", 1)
	if _, err = ParseReceipt(strings.NewReader(blank)); !errors.As(err, &ve) || ve.Field != "DespatchLineReference.LineID" {
		t.Fatalf("blank line id: err = %v", err)
	}
}
