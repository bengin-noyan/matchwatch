// Package anomaly holds the pure audit rules; it never touches the DB or network.
package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/matcher"
)

type LineContext struct {
	Line         domain.InvoiceLine
	ContractItem *domain.ContractItem
	MatchMethod  string // MatchAlias when ContractItem came from a human-confirmed alias, else "" (code/barcode)
	Suggestions  []domain.Suggestion
	Delivery     matcher.DeliveryResult
	PriceHistory []domain.PricePoint
}

const MatchAlias = "ALIAS"

// withMatch records how the contract item was found when it was not a plain code/barcode hit (D16).
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
	FXFallback          map[string]domain.ExchangeRate // TCMB rates from the invoice currency, by target (D17, D18)
}

// Run applies every rule in a fixed order: per line (in input order), then header rules.
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
