package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// fxTolerancePct: TCMB forex selling sits ~0.2% above buying and a vendor may quote a neighbouring bulletin;
// beyond 1% the invoice's own rate is no TCMB rate of that day (D18).
var fxTolerancePct = decimal.NewFromInt(1)

// rateDeviation compares the invoice's PricingExchangeRate with TCMB forex buying of the same pair (D18).
// ok=false: no invoice rate from the invoice currency, or no TCMB rate to compare with.
func rateDeviation(in Input) (ref domain.ExchangeRate, pct decimal.Decimal, ok bool) {
	r := in.Invoice.ExchangeRate
	if r == nil || r.Source != in.Invoice.Currency {
		return ref, pct, false
	}
	ref, ok = in.FXFallback[r.Target]
	if !ok || !ref.Rate.IsPositive() {
		return ref, pct, false
	}
	return ref, r.Rate.Sub(ref.Rate).Div(ref.Rate).Mul(hundred), true
}

// invoiceRateTrusted: the price rule may convert with the invoice's own rate (D4) only within the tolerance (D18).
func invoiceRateTrusted(in Input) bool {
	_, pct, ok := rateDeviation(in)
	return !ok || pct.Abs().LessThanOrEqual(fxTolerancePct)
}

// fxRateDeviation flags an invoice rate more than 1% off TCMB. Impact: the payable converted at the invoice rate
// minus at TCMB, in the target currency, only when the invoice rate makes the buyer pay more.
func fxRateDeviation(in Input) []domain.Finding {
	ref, pct, ok := rateDeviation(in)
	if !ok || pct.Abs().LessThanOrEqual(fxTolerancePct) {
		return nil
	}
	r := in.Invoice.ExchangeRate
	var impact int64
	if over := domain.FromMinor(in.Invoice.PayableMinor).Mul(r.Rate.Sub(ref.Rate)); over.IsPositive() {
		impact = domain.ToMinor(over)
	}
	return []domain.Finding{{
		Severity: domain.SeverityWarning, RuleCode: domain.RuleFXRateDeviation,
		Expected: ref.Rate.StringFixed(6), Actual: r.Rate.String(),
		ImpactMinor: impact, ImpactCurrency: r.Target,
		Evidence: map[string]any{"source_currency": r.Source, "target_currency": r.Target,
			"tcmb_rate_date": ref.TCMBDate.Format("2006-01-02"), "deviation_pct": pct.StringFixed(2),
			"tolerance_pct": fxTolerancePct.StringFixed(2)},
	}}
}
