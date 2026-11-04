package ubl

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func parseFixture(t *testing.T, name string) (*Invoice, error) {
	t.Helper()
	f, err := os.Open("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return ParseInvoice(f)
}

func date(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

func eq(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

func TestParseInvoiceBasic(t *testing.T) {
	inv, err := parseFixture(t, "invoice_basic.xml")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "ETTN", inv.ETTN, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
	eq(t, "Number", inv.Number, "ABC2026000000001")
	eq(t, "ProfileID", inv.ProfileID, "TICARIFATURA")
	eq(t, "Currency", inv.Currency, "TRY")
	if !inv.IssueDate.Equal(date("2026-03-01")) {
		t.Errorf("IssueDate = %v", inv.IssueDate)
	}
	if inv.DueDate == nil || !inv.DueDate.Equal(date("2026-03-31")) {
		t.Errorf("DueDate = %v", inv.DueDate)
	}
	if inv.ExchangeRate != nil {
		t.Errorf("ExchangeRate = %v, want nil", inv.ExchangeRate)
	}
	eq(t, "Supplier.VKN", inv.Supplier.VKN, "1234567890")
	eq(t, "Supplier.Title", inv.Supplier.Title, "Demir Bağlantı A.Ş.")
	eq(t, "Customer.VKN", inv.Customer.VKN, "9876543210")
	if len(inv.DespatchRefs) != 1 {
		t.Fatalf("DespatchRefs = %d", len(inv.DespatchRefs))
	}
	eq(t, "DespatchRef", inv.DespatchRefs[0].Number, "IRS2026000000001")
	if d := inv.DespatchRefs[0].IssueDate; d == nil || !d.Equal(date("2026-02-27")) {
		t.Errorf("DespatchRef date = %v", d)
	}
	eq(t, "TaxExclusive", inv.TaxExclusive.String(), "2300")
	eq(t, "TaxAmount", inv.TaxAmount.String(), "460")
	eq(t, "Payable", inv.Payable.String(), "2680")
	if len(inv.Lines) != 2 {
		t.Fatalf("Lines = %d", len(inv.Lines))
	}
	l := inv.Lines[0]
	eq(t, "L1.LineID", l.LineID, "1")
	eq(t, "L1.ItemCode", l.ItemCode, "CV-M8-20")
	eq(t, "L1.Barcode", l.Barcode, "8690000000017")
	eq(t, "L1.Name", l.Name, "M8 20mm Inox Civata")
	eq(t, "L1.UOM", l.UOM, "ADET")
	eq(t, "L1.Qty", l.Qty.String(), "1000")
	eq(t, "L1.UnitPrice", l.UnitPrice.String(), "1.4")
	eq(t, "L1.Allowance", l.Allowance.String(), "100")
	eq(t, "L1.LineExtension", l.LineExtension.String(), "1300")
	eq(t, "L1.VATRate", l.VATRate.String(), "20")
	eq(t, "L1.WithholdingCode", l.WithholdingCode, "")
	eq(t, "L1.WithholdingAmount", l.WithholdingAmount.String(), "0")
	l = inv.Lines[1]
	eq(t, "L2.LineID", l.LineID, "2")
	eq(t, "L2.ItemCode", l.ItemCode, "SRV-BAKIM")
	eq(t, "L2.Barcode", l.Barcode, "")
	eq(t, "L2.Name", l.Name, "Makine bakım hizmeti")
	eq(t, "L2.UnitPrice", l.UnitPrice.String(), "1000")
	eq(t, "L2.Allowance", l.Allowance.String(), "0")
	eq(t, "L2.LineExtension", l.LineExtension.String(), "1000")
	eq(t, "L2.VATRate", l.VATRate.String(), "20")
	eq(t, "L2.WithholdingCode", l.WithholdingCode, "601")
	eq(t, "L2.WithholdingPercent", l.WithholdingPercent.String(), "40")
	eq(t, "L2.WithholdingAmount", l.WithholdingAmount.String(), "80")
}

func TestParseInvoiceExchangeRate(t *testing.T) {
	inv, err := parseFixture(t, "invoice_usd.xml")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "Currency", inv.Currency, "USD")
	if inv.ExchangeRate == nil {
		t.Fatal("ExchangeRate nil")
	}
	eq(t, "Rate", inv.ExchangeRate.Rate.String(), "34.5")
	eq(t, "Source", inv.ExchangeRate.Source, "USD")
	eq(t, "Target", inv.ExchangeRate.Target, "TRY")
}

func wantValidation(t *testing.T, name, field string) {
	t.Helper()
	_, err := parseFixture(t, name)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	eq(t, "Field", ve.Field, field)
}

func TestParseInvoiceZeroQty(t *testing.T) {
	wantValidation(t, "invoice_zero_qty.xml", "InvoicedQuantity")
}
func TestParseInvoiceNoETTN(t *testing.T) { wantValidation(t, "invoice_no_ettn.xml", "UUID") }

func TestParseInvoiceRejectsDespatch(t *testing.T) {
	x := `<DespatchAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2"><ID xmlns="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">X</ID></DespatchAdvice>`
	if _, err := ParseInvoice(strings.NewReader(x)); !errors.Is(err, ErrNotInvoice) {
		t.Fatalf("err = %v, want ErrNotInvoice", err)
	}
}

func TestParseInvoiceGarbage(t *testing.T) {
	for _, in := range []string{"<x", "", "not xml"} {
		if _, err := ParseInvoice(strings.NewReader(in)); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

// Matching is by namespace URI, so arbitrary prefixes must parse identically.
func TestParseInvoiceAlternatePrefixes(t *testing.T) {
	b, err := os.ReadFile("../../testdata/invoice_basic.xml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	s = strings.NewReplacer("cac:", "a:", "cbc:", "b:", "xmlns:cac", "xmlns:a", "xmlns:cbc", "xmlns:b").Replace(s)
	s = strings.Replace(s, `xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`, `xmlns:i="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"`, 1)
	s = strings.Replace(s, "<Invoice ", "<i:Invoice ", 1)
	s = strings.Replace(s, "</Invoice>", "</i:Invoice>", 1)
	inv, err := ParseInvoice(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "Number", inv.Number, "ABC2026000000001")
	eq(t, "Payable", inv.Payable.String(), "2680")
	eq(t, "L2.WithholdingCode", inv.Lines[1].WithholdingCode, "601")
	eq(t, "Supplier.VKN", inv.Supplier.VKN, "1234567890")
}

func fixtureString(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseInvoiceCanonicalETTN(t *testing.T) {
	const canon = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	for _, in := range []string{strings.ToUpper(canon), "{" + canon + "}", "urn:uuid:" + canon, strings.ReplaceAll(canon, "-", "")} {
		x := strings.Replace(fixtureString(t, "invoice_basic.xml"), canon, in, 1)
		inv, err := ParseInvoice(strings.NewReader(x))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		eq(t, "ETTN("+in+")", inv.ETTN, canon)
	}
}

func TestParseInvoiceMultipleTaxTotals(t *testing.T) {
	x := strings.Replace(fixtureString(t, "invoice_basic.xml"), "<cac:LegalMonetaryTotal>",
		`<cac:TaxTotal><cbc:TaxAmount currencyID="TRY">10.50</cbc:TaxAmount></cac:TaxTotal><cac:LegalMonetaryTotal>`, 1)
	inv, err := ParseInvoice(strings.NewReader(x))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "TaxAmount", inv.TaxAmount.String(), "470.5")
}

func TestParseInvoiceTwoWithholdingSubtotals(t *testing.T) {
	sub := `<cac:TaxSubtotal><cbc:Percent>10</cbc:Percent><cac:TaxCategory><cac:TaxScheme><cbc:TaxTypeCode>602</cbc:TaxTypeCode></cac:TaxScheme></cac:TaxCategory></cac:TaxSubtotal>`
	x := strings.Replace(fixtureString(t, "invoice_basic.xml"), "</cac:WithholdingTaxTotal>", sub+"</cac:WithholdingTaxTotal>", 1)
	_, err := ParseInvoice(strings.NewReader(x))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	eq(t, "Field", ve.Field, "WithholdingTaxTotal")
}

func TestParseInvoiceBounds(t *testing.T) {
	base := fixtureString(t, "invoice_basic.xml")
	for name, c := range map[string]struct{ old, new, field string }{
		"amount":    {`"TRY">2300.00</cbc:TaxExclusiveAmount>`, `"TRY">1000000000000000</cbc:TaxExclusiveAmount>`, "TaxExclusiveAmount"},
		"qty":       {`unitCode="C62">1000<`, `unitCode="C62">1000000000000000<`, "InvoicedQuantity"},
		"price":     {">1.40<", ">10000000000000<", "PriceAmount"},
		"exponent":  {`"TRY">2300.00</cbc:TaxExclusiveAmount>`, `"TRY">1e999999999</cbc:TaxExclusiveAmount>`, "TaxExclusiveAmount"},
		"empty cur": {">TRY</cbc:DocumentCurrencyCode>", "></cbc:DocumentCurrencyCode>", "DocumentCurrencyCode"},
		"lower cur": {">TRY</cbc:DocumentCurrencyCode>", ">try</cbc:DocumentCurrencyCode>", "DocumentCurrencyCode"},
	} {
		_, err := ParseInvoice(strings.NewReader(strings.Replace(base, c.old, c.new, 1)))
		var ve *ValidationError
		if !errors.As(err, &ve) || (c.field != "" && ve.Field != c.field) {
			t.Errorf("%s: err = %v, want ValidationError on %s", name, err, c.field)
		}
	}
}

func TestParseInvoiceBadExchangeRate(t *testing.T) {
	b, err := os.ReadFile("../../testdata/invoice_usd.xml")
	if err != nil {
		t.Fatal(err)
	}
	for field, repl := range map[string][2]string{
		"SourceCurrencyCode": {"<cbc:SourceCurrencyCode>USD</cbc:SourceCurrencyCode>", "<cbc:SourceCurrencyCode>usd</cbc:SourceCurrencyCode>"},
		"TargetCurrencyCode": {"<cbc:TargetCurrencyCode>TRY</cbc:TargetCurrencyCode>", "<cbc:TargetCurrencyCode>TL</cbc:TargetCurrencyCode>"},
		"CalculationRate":    {"<cbc:CalculationRate>34.500000</cbc:CalculationRate>", "<cbc:CalculationRate>0</cbc:CalculationRate>"},
	} {
		if !strings.Contains(string(b), repl[0]) {
			t.Fatalf("fixture lacks %q", repl[0])
		}
		_, err := ParseInvoice(strings.NewReader(strings.Replace(string(b), repl[0], repl[1], 1)))
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != field {
			t.Errorf("%s: err = %v", field, err)
		}
	}
}
