package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// fxTolerancePct: TCMB'nin satış kuru alış kurundan yaklaşık %0,2 yüksek. Tedarikçi bir önceki ya da sonraki günün
// kurunu da kullanmış olabilir. Bu yüzden %1'e kadar farkı normal sayıyoruz; daha fazlası o günün TCMB kuru olamaz
// (D18).
var fxTolerancePct = decimal.NewFromInt(1)

// rateDeviation, faturanın kendi kurunu (PricingExchangeRate) aynı döviz çifti için TCMB döviz alış kuruyla
// karşılaştırır (D18). Faturada fatura para biriminden çıkan bir kur yoksa ya da karşılaştıracak TCMB kuru yoksa
// ok=false döner.
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

// invoiceRateTrusted: fiyat kuralı faturanın kendi kurunu (D4) ancak fark tolerans içindeyse kullanabilir (D18).
func invoiceRateTrusted(in Input) bool {
	_, pct, ok := rateDeviation(in)
	return !ok || pct.Abs().LessThanOrEqual(fxTolerancePct)
}

// fxRateDeviation, faturanın kuru TCMB'den %1'den fazla saparsa bulgu yazar. Etki: ödenecek tutarın fatura kuruyla
// çevrilmiş hali eksi TCMB kuruyla çevrilmiş hali, hedef para biriminde. Fark yalnızca alıcı fazla ödüyorsa etki
// sayılır.
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
