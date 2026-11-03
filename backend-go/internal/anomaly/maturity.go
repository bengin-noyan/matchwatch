package anomaly

import (
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// calendarDays counts whole days between the calendar dates of a and b, ignoring zone/DST.
func calendarDays(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return int(time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC).Sub(time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
}

func maturityViolation(in Input) []domain.Finding {
	if in.Contract == nil || in.Invoice.DueDate == nil {
		return nil
	}
	days := calendarDays(in.Invoice.IssueDate, *in.Invoice.DueDate)
	term := in.Contract.PaymentTermDays
	if days >= term {
		return nil
	}
	short := decimal.NewFromInt(int64(term - days))
	impact := domain.FromMinor(in.Invoice.PayableMinor).Mul(in.FinancingRateAnnual).Mul(short).
		DivRound(decimal.NewFromInt(365), 10)
	return []domain.Finding{{
		Severity: domain.SeverityWarning, RuleCode: domain.RuleMaturityViolation,
		Expected: strconv.Itoa(term) + " days", Actual: strconv.Itoa(days) + " days",
		ImpactMinor: domain.ToMinor(impact), ImpactCurrency: in.Invoice.Currency,
		Evidence: map[string]any{"contract_no": in.Contract.Number, "payment_term_days": term,
			"actual_days": days, "financing_rate_annual": in.FinancingRateAnnual.String(),
			"source_page": in.Contract.PaymentTermSourcePage, "source_clause_text": in.Contract.PaymentTermClauseText},
	}}
}
