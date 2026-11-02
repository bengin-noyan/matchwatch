package anomaly

import (
	"testing"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func TestPriceMismatch(t *testing.T) {
	fs := Run(baseInput(baseCtx("1000", "1300")))
	if len(fs) != 1 {
		t.Fatalf("findings=%d %+v", len(fs), fs)
	}
	f := fs[0]
	if f.RuleCode != domain.RulePriceMismatch || f.Severity != domain.SeverityCritical || f.ImpactMinor != 5000 ||
		f.Expected != "1.250000 TRY" || f.Actual != "1.300000 TRY" || f.ImpactCurrency != "TRY" {
		t.Errorf("%+v", f)
	}
	if f.Evidence["source_page"] != 3 || f.Evidence["line_id"] != "1" || f.Evidence["contract_no"] != "C-1" || f.InvoiceLineID == nil {
		t.Errorf("evidence %+v", f.Evidence)
	}
}

func TestPriceMismatchUsesNetOfAllowance(t *testing.T) {
	lc := baseCtx("1000", "1300")
	lc.Line.UnitPrice = dec("1.40")
	lc.Line.AllowanceAmount = dec("100")
	fs := only(t, Run(baseInput(lc)), domain.RulePriceMismatch)
	if len(fs) != 1 || fs[0].ImpactMinor != 5000 {
		t.Errorf("%+v", fs)
	}
}

func TestPriceEqualOrLowerNoFinding(t *testing.T) {
	for _, ext := range []string{"1250", "1200"} {
		if fs := only(t, Run(baseInput(baseCtx("1000", ext))), domain.RulePriceMismatch); len(fs) != 0 {
			t.Errorf("ext %s: %+v", ext, fs)
		}
	}
}

func TestPriceFXConverted(t *testing.T) {
	lc := baseCtx("10", "1.00")
	lc.ContractItem.UnitPrice = dec("3.000000")
	in := baseInput(lc)
	in.Invoice.Currency = "USD"
	in.Invoice.ExchangeRate = &domain.ExchangeRate{Source: "USD", Target: "TRY", Rate: dec("34.5")}
	fs := only(t, Run(in), domain.RulePriceMismatch)
	if len(fs) != 1 {
		t.Fatalf("%+v", fs)
	}
	f := fs[0]
	if f.ImpactMinor != 450 || f.ImpactCurrency != "TRY" || f.Evidence["fx_rate"] != "34.5" || f.Actual != "3.450000 TRY" {
		t.Errorf("%+v", f)
	}
}

func TestPriceFXUnverified(t *testing.T) {
	lc := baseCtx("10", "100")
	lc.ContractItem.Currency = "USD"
	fs := Run(baseInput(lc))
	if len(fs) != 1 || fs[0].RuleCode != domain.RuleFXUnverified || fs[0].Severity != domain.SeverityInfo ||
		fs[0].Evidence["contract_no"] != "C-1" || fs[0].Evidence["source_page"] != 3 {
		t.Errorf("%+v", fs)
	}
}

func TestUOMMismatch(t *testing.T) {
	lc := baseCtx("10", "100")
	lc.ContractItem.UOM = "KG"
	fs := Run(baseInput(lc))
	if len(fs) != 1 || fs[0].RuleCode != domain.RuleUOMMismatch || fs[0].Severity != domain.SeverityWarning ||
		fs[0].Evidence["contract_no"] != "C-1" || fs[0].Evidence["source_page"] != 3 {
		t.Errorf("%+v", fs)
	}
}

func TestUncontractedItem(t *testing.T) {
	lc := baseCtx("10", "100")
	lc.ContractItem = nil
	lc.Delivery.CandidateCount = 0
	fs := Run(baseInput(lc))
	if len(fs) != 1 || fs[0].RuleCode != domain.RuleUncontractedItem || fs[0].Severity != domain.SeverityWarning || fs[0].ImpactMinor != 0 {
		t.Errorf("%+v", fs)
	}
}

func TestPriceSubKurusNoFinding(t *testing.T) {
	lc := baseCtx("3000", "1000")
	lc.ContractItem.UnitPrice = dec("0.333333")
	if fs := Run(baseInput(lc)); len(fs) != 0 {
		t.Errorf("%+v", fs)
	}
}
