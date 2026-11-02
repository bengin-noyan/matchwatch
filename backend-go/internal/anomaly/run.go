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
	Delivery     matcher.DeliveryResult
	PriceHistory []domain.PricePoint
}

type Input struct {
	Invoice             domain.Invoice
	Contract            *domain.Contract
	Lines               []LineContext
	Duplicates          []domain.DuplicateCandidate
	WithholdingPercents map[string]decimal.Decimal
	FinancingRateAnnual decimal.Decimal
}

// Run applies every rule in a fixed order: per line (in input order), then header rules.
func Run(in Input) []domain.Finding {
	var out []domain.Finding
	for _, lc := range in.Lines {
		out = append(out, uncontractedItem(in, lc)...)
		out = append(out, priceRules(in, lc)...)
		out = append(out, qtyExceeds(in, lc)...)
	}
	out = append(out, maturityViolation(in)...)
	return out
}
