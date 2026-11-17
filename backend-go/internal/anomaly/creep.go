package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

var (
	creepAlpha     = decimal.NewFromInt(2).Div(decimal.NewFromInt(7))
	creepThreshold = decimal.RequireFromString("1.10")
)

// priceCreep, sözleşmede olmayan bir kalemin fiyatı önceki fiyatların ortalamasını (EMA) %10'dan fazla aşmışsa bulgu
// yazar. Geçmiş fiyatlar eskiden yeniye sıralı gelir.
func priceCreep(in Input, lc LineContext) []domain.Finding {
	h := lc.PriceHistory
	if lc.ContractItem != nil || len(h) < 3 {
		return nil
	}
	ema := h[0].NetUnitPrice
	for _, p := range h[1:] {
		ema = ema.Add(creepAlpha.Mul(p.NetUnitPrice.Sub(ema)))
	}
	ema = ema.Round(6)
	net := lc.Line.NetUnitPrice()
	if !net.GreaterThan(ema.Mul(creepThreshold)) {
		return nil
	}
	impact := domain.ToMinor(net.Sub(ema).Mul(lc.Line.Qty))
	if impact <= 0 {
		return nil
	}
	return []domain.Finding{{
		InvoiceLineID: &lc.Line.ID, Severity: domain.SeverityWarning, RuleCode: domain.RulePriceCreep,
		Expected: ema.String(), Actual: net.String(), ImpactMinor: impact, ImpactCurrency: in.Invoice.Currency,
		Evidence: map[string]any{"line_id": lc.Line.LineID, "ema": ema.String(), "history_count": len(h)},
	}}
}
