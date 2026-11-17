package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

var (
	hundred   = decimal.NewFromInt(100)
	halfKurus = decimal.New(5, -3) // 0,005'e kadar olan etkiler yuvarlamadan gelir
)

// qtyExceeds, faturalanan miktarı irsaliyeyle teslim alınan miktarla karşılaştırır. Hizmet kalemleri bu kurala girmez
// (D14).
func qtyExceeds(in Input, lc LineContext) []domain.Finding {
	ci, line := lc.ContractItem, lc.Line
	if ci != nil && ci.UOM != line.UOM {
		return nil // birim tutmuyorsa miktar kuralını atlıyoruz (D5)
	}
	if !((ci != nil && ci.RequiresDelivery) || (ci == nil && in.Invoice.HasDespatchRefs)) {
		return nil
	}
	tol := decimal.Zero
	if in.Contract != nil {
		tol = in.Contract.QtyTolerancePct
	}
	limit := lc.Delivery.AvailableTotal.Mul(decimal.NewFromInt(1).Add(tol.Div(hundred)))
	if !line.Qty.GreaterThan(limit) {
		return nil
	}
	exact := line.Qty.Sub(lc.Delivery.AvailableTotal).Mul(line.NetUnitPrice())
	if !exact.GreaterThan(halfKurus) {
		return nil // en fazla yarım kuruşluk fark yuvarlamadan gelir
	}
	impact := domain.ToMinor(exact)
	allocs := make([]string, 0, len(lc.Delivery.Allocations))
	for _, a := range lc.Delivery.Allocations {
		allocs = append(allocs, a.DespatchLineID.String())
	}
	ev := map[string]any{"line_id": line.LineID, "available_total": lc.Delivery.AvailableTotal.String(),
		"tolerance_pct": tol.String(), "allocations": allocs}
	withMatch(ev, lc)
	if lc.Delivery.CandidateCount == 0 {
		ev["no_despatch_found"] = true
	}
	return []domain.Finding{{
		InvoiceLineID: &line.ID, Severity: domain.SeverityCritical, RuleCode: domain.RuleQtyExceedsDelivery,
		Expected: lc.Delivery.AvailableTotal.String() + " " + line.UOM, Actual: line.Qty.String() + " " + line.UOM,
		ImpactMinor:    impact,
		ImpactCurrency: in.Invoice.Currency, Evidence: ev,
	}}
}
