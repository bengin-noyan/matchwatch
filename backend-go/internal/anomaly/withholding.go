package anomaly

import (
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func withholdingError(in Input, lc LineContext) []domain.Finding {
	line := lc.Line
	expected := ""
	if lc.ContractItem != nil {
		expected = lc.ContractItem.WithholdingCode
	}
	pct := decimal.Zero // etkiyi hesaplarken beklenen oranı kullanıyoruz
	reason := ""
	linePct, lineKnown := in.WithholdingPercents[line.WithholdingCode]
	switch {
	case expected != "" && line.WithholdingCode != expected:
		reason, pct = "code_mismatch", in.WithholdingPercents[expected]
	case line.WithholdingCode != "" && !lineKnown:
		reason = "unknown_code"
	case line.WithholdingCode != "" && !line.WithholdingPercent.Equal(linePct):
		reason, pct = "percent_mismatch", linePct
	default:
		return nil
	}
	want := line.LineExtension.Mul(line.VATRate).Div(hundred).Mul(pct).Div(hundred)
	impact := domain.ToMinor(want.Sub(line.WithholdingAmount).Abs())
	if impact <= 0 && reason != "unknown_code" {
		return nil
	}
	ev := map[string]any{"line_id": line.LineID, "reason": reason,
		"expected_code": expected, "actual_code": line.WithholdingCode}
	withMatch(ev, lc)
	if lc.ContractItem != nil {
		ev["source_page"], ev["source_clause_text"] = lc.ContractItem.SourcePage, lc.ContractItem.SourceClauseText
		if in.Contract != nil {
			ev["contract_no"] = in.Contract.Number
		}
	}
	return []domain.Finding{{
		InvoiceLineID: &line.ID, Severity: domain.SeverityCritical, RuleCode: domain.RuleWithholdingError,
		Expected: expected, Actual: line.WithholdingCode, ImpactMinor: impact,
		ImpactCurrency: in.Invoice.Currency, Evidence: ev,
	}}
}
