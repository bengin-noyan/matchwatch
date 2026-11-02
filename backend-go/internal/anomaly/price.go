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
	return []domain.Finding{{
		InvoiceLineID: &lc.Line.ID, Severity: domain.SeverityWarning, RuleCode: domain.RuleUncontractedItem,
		ImpactCurrency: in.Invoice.Currency,
		Evidence: map[string]any{"line_id": lc.Line.LineID, "item_code": lc.Line.ItemCode,
			"contract_no": in.Contract.Number},
	}}
}

// priceRules runs the uom and fx pre-checks, then the price comparison (D2, D4, D5).
func priceRules(in Input, lc LineContext) []domain.Finding {
	ci, line := lc.ContractItem, lc.Line
	if ci == nil {
		return nil
	}
	if ci.UOM != line.UOM {
		return []domain.Finding{{
			InvoiceLineID: &line.ID, Severity: domain.SeverityWarning, RuleCode: domain.RuleUOMMismatch,
			Expected: ci.UOM, Actual: line.UOM, ImpactCurrency: in.Invoice.Currency,
			Evidence: map[string]any{"line_id": line.LineID, "contract_item_uom": ci.UOM, "invoice_uom": line.UOM},
		}}
	}
	net := line.NetUnitPrice()
	evidence := map[string]any{"line_id": line.LineID, "source_page": ci.SourcePage,
		"source_clause_text": ci.SourceClauseText}
	if in.Contract != nil {
		evidence["contract_no"] = in.Contract.Number
	}
	if in.Invoice.Currency != ci.Currency {
		r := in.Invoice.ExchangeRate
		if r == nil || r.Source != in.Invoice.Currency || r.Target != ci.Currency {
			return []domain.Finding{{
				InvoiceLineID: &line.ID, Severity: domain.SeverityInfo, RuleCode: domain.RuleFXUnverified,
				ImpactCurrency: in.Invoice.Currency,
				Evidence: map[string]any{"line_id": line.LineID, "invoice_currency": in.Invoice.Currency,
					"contract_currency": ci.Currency},
			}}
		}
		net = net.Mul(r.Rate)
		evidence["fx_rate"] = r.Rate.String()
	}
	if !net.GreaterThan(ci.UnitPrice) {
		return nil
	}
	return []domain.Finding{{
		InvoiceLineID: &line.ID, Severity: domain.SeverityCritical, RuleCode: domain.RulePriceMismatch,
		Expected: money(ci.UnitPrice, ci.Currency), Actual: money(net, ci.Currency),
		ImpactMinor:    domain.ToMinor(net.Sub(ci.UnitPrice).Mul(line.Qty)),
		ImpactCurrency: ci.Currency, Evidence: evidence,
	}}
}
