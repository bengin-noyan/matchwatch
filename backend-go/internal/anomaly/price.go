package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func money(d decimal.Decimal, cur string) string { return d.StringFixed(6) + " " + cur }

func uncontractedItem(in Input, lc LineContext) []domain.Finding {
	if in.Contract == nil || lc.ContractItem != nil {
		return nil
	}
	ev := map[string]any{"line_id": lc.Line.LineID, "item_code": lc.Line.ItemCode, "contract_no": in.Contract.Number}
	if len(lc.Suggestions) > 0 {
		sug := make([]map[string]any, len(lc.Suggestions))
		for i, s := range lc.Suggestions {
			sug[i] = map[string]any{"contract_item_id": s.ContractItemID.String(), "item_name": s.ItemName,
				"similarity": s.Similarity.StringFixed(4)}
		}
		ev["suggestions"] = sug
	}
	return []domain.Finding{{
		InvoiceLineID: &lc.Line.ID, Severity: domain.SeverityWarning, RuleCode: domain.RuleUncontractedItem,
		ImpactCurrency: in.Invoice.Currency, Evidence: ev,
	}}
}

// priceRules önce birimi ve kuru kontrol eder, sonra fiyatı karşılaştırır (D2, D4, D5).
func priceRules(in Input, lc LineContext) []domain.Finding {
	ci, line := lc.ContractItem, lc.Line
	if ci == nil {
		return nil
	}
	evidence := withMatch(map[string]any{"line_id": line.LineID, "source_page": ci.SourcePage,
		"source_clause_text": ci.SourceClauseText}, lc)
	if in.Contract != nil {
		evidence["contract_no"] = in.Contract.Number
	}
	if ci.UOM != line.UOM {
		evidence["contract_item_uom"], evidence["invoice_uom"] = ci.UOM, line.UOM
		return []domain.Finding{{
			InvoiceLineID: &line.ID, Severity: domain.SeverityWarning, RuleCode: domain.RuleUOMMismatch,
			Expected: ci.UOM, Actual: line.UOM, ImpactCurrency: in.Invoice.Currency, Evidence: evidence,
		}}
	}
	net := line.NetUnitPrice()
	if in.Invoice.Currency != ci.Currency {
		r := in.Invoice.ExchangeRate
		if r == nil || r.Source != in.Invoice.Currency || r.Target != ci.Currency || !invoiceRateTrusted(in) {
			r = nil
			if fb, ok := in.FXFallback[ci.Currency]; ok { // D17: faturanın kendi kuru öncelikli (D4), ama TCMB'den çok saparsa kullanılmaz (D18)
				r = &fb
				evidence["fx_source"], evidence["fx_rate_date"] = "TCMB", fb.TCMBDate.Format("2006-01-02")
			}
		}
		if r == nil {
			return []domain.Finding{{
				InvoiceLineID: &line.ID, Severity: domain.SeverityInfo, RuleCode: domain.RuleFXUnverified,
				ImpactCurrency: in.Invoice.Currency,
				Evidence:       fxEvidence(evidence, in.Invoice.Currency, ci.Currency),
			}}
		}
		net = net.Mul(r.Rate)
		evidence["fx_rate"] = r.Rate.String()
	}
	if !net.GreaterThan(ci.UnitPrice) {
		return nil
	}
	exact := net.Sub(ci.UnitPrice).Mul(line.Qty)
	if !exact.GreaterThan(halfKurus) {
		return nil // en fazla yarım kuruşluk fark yuvarlamadan gelir (satır toplamları tam kuruş saklanıyor)
	}
	impact := domain.ToMinor(exact)
	return []domain.Finding{{
		InvoiceLineID: &line.ID, Severity: domain.SeverityCritical, RuleCode: domain.RulePriceMismatch,
		Expected: money(ci.UnitPrice, ci.Currency), Actual: money(net, ci.Currency),
		ImpactMinor:    impact,
		ImpactCurrency: ci.Currency, Evidence: evidence,
	}}
}

func fxEvidence(ev map[string]any, invCur, conCur string) map[string]any {
	ev["invoice_currency"], ev["contract_currency"] = invCur, conCur
	return ev
}
