package ubl

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseDespatchBasic(t *testing.T) {
	d, err := ParseDespatch(strings.NewReader(readFixture(t, "despatch_basic.xml")))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "ETTN", d.ETTN, "0b0c5c4e-6d43-4b5a-9f39-3c1f0c0e9a11")
	eq(t, "Number", d.Number, "IRS2026000000001")
	if !d.IssueDate.Equal(date("2026-02-27")) {
		t.Errorf("IssueDate = %v", d.IssueDate)
	}
	if d.ShipmentDate == nil || !d.ShipmentDate.Equal(date("2026-02-27")) {
		t.Errorf("ShipmentDate = %v", d.ShipmentDate)
	}
	eq(t, "Supplier.VKN", d.Supplier.VKN, "1234567890")
	eq(t, "Supplier.Title", d.Supplier.Title, "Demir Bağlantı A.Ş.")
	eq(t, "Customer.VKN", d.Customer.VKN, "9876543210")
	if len(d.Lines) != 1 {
		t.Fatalf("Lines = %d", len(d.Lines))
	}
	l := d.Lines[0]
	eq(t, "LineID", l.LineID, "1")
	eq(t, "ItemCode", l.ItemCode, "CV-M8-20")
	eq(t, "Barcode", l.Barcode, "8690000000017")
	eq(t, "Name", l.Name, "Civata M8x20 Paslanmaz")
	eq(t, "UOM", l.UOM, "ADET")
	eq(t, "DeliveredQty", l.DeliveredQty.String(), "1000")
}

func TestParseDespatchRejectsInvoice(t *testing.T) {
	if _, err := ParseDespatch(strings.NewReader(readFixture(t, "invoice_basic.xml"))); !errors.Is(err, ErrNotDespatch) {
		t.Fatalf("err = %v, want ErrNotDespatch", err)
	}
}

func TestParseDespatchNegativeQty(t *testing.T) {
	base := readFixture(t, "despatch_basic.xml")
	if _, err := ParseDespatch(strings.NewReader(strings.Replace(base, ">1000<", ">0<", 1))); err != nil {
		t.Errorf("zero qty must be allowed: %v", err)
	}
	_, err := ParseDespatch(strings.NewReader(strings.Replace(base, ">1000<", ">-1<", 1)))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	eq(t, "Field", ve.Field, "DeliveredQuantity")
}

func TestParseDespatchValidation(t *testing.T) {
	base := readFixture(t, "despatch_basic.xml")
	for field, bad := range map[string]string{
		"UUID":         strings.Replace(base, "0b0c5c4e-6d43-4b5a-9f39-3c1f0c0e9a11", "nope", 1),
		"Supplier.VKN": strings.Replace(base, "1234567890", "123", 1),
	} {
		_, err := ParseDespatch(strings.NewReader(bad))
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != field {
			t.Errorf("%s: err = %v", field, err)
		}
	}
}
