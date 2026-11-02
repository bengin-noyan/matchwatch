package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

var hundred = decimal.NewFromInt(100)

// qtyExceeds compares invoiced qty with despatched qty (D14: service lines are exempt).
func qtyExceeds(in Input, lc LineContext) []domain.Finding {
	ci, line := lc.ContractItem, lc.Line
	if ci != nil && ci.UOM != line.UOM {
		return nil // uom mismatch: qty rule skipped (D5)
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
	impact := domain.ToMinor(line.Qty.Sub(lc.Delivery.AvailableTotal).Mul(line.NetUnitPrice()))
	if impact <= 0 {
		return nil // below one kurus: rounding noise
	}
	allocs := make([]string, 0, len(lc.Delivery.Allocations))
	for _, a := range lc.Delivery.Allocations {
		allocs = append(allocs, a.DespatchLineID.String())
	}
	ev := map[string]any{"line_id": line.LineID, "available_total": lc.Delivery.AvailableTotal.String(),
		"tolerance_pct": tol.String(), "allocations": allocs}
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
