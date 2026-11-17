// Package anomaly, denetim kurallarını içerir. Kurallar saf fonksiyonlardır: veritabanına ya da ağa hiç dokunmazlar.
package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/matcher"
)

type LineContext struct {
	Line         domain.InvoiceLine
	ContractItem *domain.ContractItem
	MatchMethod  string // sözleşme kalemi bir insanın onayladığı eşleşmeden geldiyse MatchAlias, kod ya da barkoddan geldiyse ""
	Suggestions  []domain.Suggestion
	Delivery     matcher.DeliveryResult
	PriceHistory []domain.PricePoint
}

const MatchAlias = "ALIAS"

// withMatch, sözleşme kalemi düz bir kod ya da barkod eşleşmesiyle bulunmadıysa nasıl bulunduğunu kanıta yazar (D16).
func withMatch(ev map[string]any, lc LineContext) map[string]any {
	if lc.MatchMethod != "" {
		ev["match_method"] = lc.MatchMethod
	}
	return ev
}

type Input struct {
	Invoice             domain.Invoice
	Contract            *domain.Contract
	Lines               []LineContext
	Duplicates          []domain.DuplicateCandidate
	WithholdingPercents map[string]decimal.Decimal
	FinancingRateAnnual decimal.Decimal
	FXFallback          map[string]domain.ExchangeRate // fatura para biriminden hedef para birimine TCMB kurları (D17, D18)
}

// Run, bütün kuralları sabit bir sırayla çalıştırır: önce her satır için (geldikleri sırayla), sonra fatura geneli
// için.
func Run(in Input) []domain.Finding {
	var out []domain.Finding
	for _, lc := range in.Lines {
		out = append(out, uncontractedItem(in, lc)...)
		out = append(out, priceRules(in, lc)...)
		out = append(out, qtyExceeds(in, lc)...)
		out = append(out, withholdingError(in, lc)...)
		out = append(out, priceCreep(in, lc)...)
	}
	out = append(out, maturityViolation(in)...)
	out = append(out, duplicateSuspect(in)...)
	out = append(out, fxRateDeviation(in)...)
	return out
}
