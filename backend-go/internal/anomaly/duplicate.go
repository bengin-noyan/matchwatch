package anomaly

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func tokenSet(names []string) map[string]bool {
	set := map[string]bool{}
	for _, n := range names {
		low := strings.ToLowerSpecial(unicode.TurkishCase, n)
		for _, tok := range strings.FieldsFunc(low, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			set[tok] = true
		}
	}
	return set
}

// tokenJaccard, iki faturadaki kalem adlarının ne kadar benzediğini ölçer: ortak kelime sayısı / toplam farklı kelime
// sayısı. Kelimeler Türkçe kurallarıyla küçük harfe çevrilir.
func tokenJaccard(a, b []string) decimal.Decimal {
	sa, sb := tokenSet(a), tokenSet(b)
	inter := 0
	for t := range sa {
		if sb[t] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(inter)).Div(decimal.NewFromInt(int64(union)))
}

func duplicateSuspect(in Input) []domain.Finding {
	var names []string
	for _, l := range in.Invoice.Lines {
		names = append(names, l.Name)
	}
	threshold := decimal.RequireFromString("0.80")
	var ids []string
	reason := ""
	for _, c := range in.Duplicates {
		switch {
		case c.Number == in.Invoice.Number:
			reason = "same_number" // aynı fatura numarası, tutar eşleşmesinden önce gelir
		case c.PayableMinor == in.Invoice.PayableMinor && tokenJaccard(names, c.LineNames).GreaterThanOrEqual(threshold):
			if reason == "" {
				reason = "same_amount_similar_items"
			}
		default:
			continue
		}
		ids = append(ids, c.InvoiceID.String())
	}
	if len(ids) == 0 {
		return nil
	}
	return []domain.Finding{{
		Severity: domain.SeverityCritical, RuleCode: domain.RuleDuplicateSuspect,
		ImpactMinor: in.Invoice.PayableMinor, ImpactCurrency: in.Invoice.Currency,
		Evidence: map[string]any{"candidate_invoice_ids": ids, "match_reason": reason},
	}}
}
