package anomaly

import (
	"testing"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// usdInvoice: 1000 units at 1.30 USD against a TRY contract price, payable 100.00 USD, TCMB USD/TRY 40.
func usdInvoice(rate, contractTRY string) Input {
	lc := baseCtx("1000", "1300")
	lc.ContractItem.UnitPrice = dec(contractTRY)
	in := baseInput(lc)
	in.Invoice.Currency, in.Invoice.PayableMinor = "USD", 10000
	in.Invoice.ExchangeRate = &domain.ExchangeRate{Source: "USD", Target: "TRY", Rate: dec(rate)}
	in.FXFallback = map[string]domain.ExchangeRate{"TRY": {Source: "USD", Target: "TRY", Rate: dec("40"), TCMBDate: date("2026-02-27")}}
	return in
}

func TestFXRateDeviationInflated(t *testing.T) {
	fs := Run(usdInvoice("42", "52.000000")) // at TCMB 1.30 * 40 = 52: the price is right, only the rate is not
	if len(fs) != 1 {
		t.Fatalf("%+v", fs)
	}
	f := fs[0]
	if f.RuleCode != domain.RuleFXRateDeviation || f.Severity != domain.SeverityWarning || f.InvoiceLineID != nil ||
		f.ImpactMinor != 20000 || f.ImpactCurrency != "TRY" || f.Expected != "40.000000" || f.Actual != "42" {
		t.Errorf("%+v", f) // 100 USD * (42 - 40) = 200.00 TRY
	}
	if f.Evidence["deviation_pct"] != "5.00" || f.Evidence["tcmb_rate_date"] != "2026-02-27" || f.Evidence["target_currency"] != "TRY" {
		t.Errorf("evidence %+v", f.Evidence)
	}
}

func TestFXRateDeviationUnderstatedRevealsPrice(t *testing.T) {
	fs := Run(usdInvoice("38", "51.000000")) // at 38 the line looks like 49.40 TRY; at TCMB it is 52 > 51
	p, d := only(t, fs, domain.RulePriceMismatch), only(t, fs, domain.RuleFXRateDeviation)
	if len(p) != 1 || p[0].ImpactMinor != 100000 || p[0].Evidence["fx_source"] != "TCMB" || p[0].Evidence["fx_rate"] != "40" {
		t.Fatalf("price %+v", p)
	}
	if len(d) != 1 || d[0].ImpactMinor != 0 || d[0].Evidence["deviation_pct"] != "-5.00" {
		t.Fatalf("fx %+v", d)
	}
}

func TestFXRateWithinToleranceIsTrusted(t *testing.T) {
	fs := Run(usdInvoice("40.3", "52.000000")) // 0.75% off: 1.30 * 40.3 = 52.39 -> 0.39 * 1000 = 390.00 TRY
	if len(fs) != 1 || fs[0].RuleCode != domain.RulePriceMismatch || fs[0].ImpactMinor != 39000 || fs[0].Evidence["fx_rate"] != "40.3" {
		t.Fatalf("%+v", fs)
	}
}

func TestFXRateDeviationNeedsTCMB(t *testing.T) {
	in := usdInvoice("42", "60.000000")
	in.FXFallback = nil
	if fs := Run(in); len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}
