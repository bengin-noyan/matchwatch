package anomaly

import (
	"testing"

	"github.com/google/uuid"

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

func TestPriceHalfKurusIsNoise(t *testing.T) {
	lc := baseCtx("2", "0.08")                // net 0.04
	lc.ContractItem.UnitPrice = dec("0.0375") // impact exactly 0.005
	if fs := only(t, Run(baseInput(lc)), domain.RulePriceMismatch); len(fs) != 0 {
		t.Errorf("half kurus: %+v", fs)
	}
	lc.ContractItem.UnitPrice = dec("0.0374") // impact 0.0052
	if fs := only(t, Run(baseInput(lc)), domain.RulePriceMismatch); len(fs) != 1 || fs[0].ImpactMinor != 1 {
		t.Errorf("just above: %+v", fs)
	}
}

func TestUncontractedItemCarriesSuggestions(t *testing.T) {
	lc := baseCtx("10", "100")
	lc.ContractItem = nil
	lc.Delivery.CandidateCount = 0
	id := uuid.New()
	lc.Suggestions = []domain.Suggestion{{ContractItemID: id, ItemName: "Ayçiçek Yağı 5 L", Similarity: dec("0.87")}}
	fs := Run(baseInput(lc))
	sug, ok := fs[0].Evidence["suggestions"].([]map[string]any)
	if len(fs) != 1 || !ok || len(sug) != 1 || sug[0]["contract_item_id"] != id.String() ||
		sug[0]["item_name"] != "Ayçiçek Yağı 5 L" || sug[0]["similarity"] != "0.8700" {
		t.Fatalf("%+v", fs)
	}
}

func TestAliasMatchMarksEvidence(t *testing.T) {
	lc := baseCtx("10", "200") // net 20 > 1.25
	lc.MatchMethod = MatchAlias
	fs := Run(baseInput(lc))
	if len(fs) != 1 || fs[0].RuleCode != domain.RulePriceMismatch || fs[0].Evidence["match_method"] != "ALIAS" {
		t.Fatalf("%+v", fs)
	}
	lc.MatchMethod = ""
	if fs := Run(baseInput(lc)); fs[0].Evidence["match_method"] != nil {
		t.Fatalf("code match marked: %+v", fs[0].Evidence)
	}
}

func usdLine() (Input, LineContext) {
	lc := baseCtx("10", "200") // TRY net 20.00
	lc.ContractItem.Currency, lc.ContractItem.UnitPrice = "USD", dec("0.390000")
	in := baseInput(lc)
	return in, lc
}

func TestPriceUsesTCMBFallback(t *testing.T) {
	in, _ := usdLine()
	in.FXFallback = map[string]domain.ExchangeRate{"USD": {Source: "TRY", Target: "USD", Rate: dec("0.02"), TCMBDate: date("2026-02-27")}}
	fs := Run(in) // 20.00 TRY * 0.02 = 0.40 USD > 0.39
	if len(fs) != 1 || fs[0].RuleCode != domain.RulePriceMismatch || fs[0].ImpactMinor != 10 || fs[0].ImpactCurrency != "USD" ||
		fs[0].Evidence["fx_source"] != "TCMB" || fs[0].Evidence["fx_rate_date"] != "2026-02-27" || fs[0].Evidence["fx_rate"] != "0.02" {
		t.Fatalf("%+v", fs)
	}
}

func TestPriceInvoiceRateBeatsTCMB(t *testing.T) {
	in, _ := usdLine()
	in.Invoice.ExchangeRate = &domain.ExchangeRate{Source: "TRY", Target: "USD", Rate: dec("0.0199")} // 0.5% off: trusted (D18)
	in.FXFallback = map[string]domain.ExchangeRate{"USD": {Source: "TRY", Target: "USD", Rate: dec("0.02"), TCMBDate: date("2026-02-27")}}
	fs := Run(in) // 20 * 0.0199 = 0.398 vs 0.39 -> 0.08 USD (TCMB would give 0.10)
	if len(fs) != 1 || fs[0].ImpactMinor != 8 || fs[0].Evidence["fx_rate"] != "0.0199" || fs[0].Evidence["fx_source"] != nil {
		t.Fatalf("%+v", fs)
	}
}

func TestPriceNoRateStillUnverified(t *testing.T) {
	in, _ := usdLine()
	in.FXFallback = map[string]domain.ExchangeRate{"EUR": {Rate: dec("0.02")}}
	if fs := Run(in); len(fs) != 1 || fs[0].RuleCode != domain.RuleFXUnverified {
		t.Fatalf("%+v", fs)
	}
}
