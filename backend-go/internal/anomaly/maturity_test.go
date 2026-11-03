package anomaly

import (
	"testing"
	"time"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func matInput(due string) Input {
	in := baseInput(baseCtx("1000", "1250"))
	in.Invoice.PayableMinor = 10000000
	in.FinancingRateAnnual = dec("0.4500")
	if due != "" {
		d := date(due)
		in.Invoice.DueDate = &d
	}
	return in
}

func TestMaturityViolation(t *testing.T) {
	fs := only(t, Run(matInput("2026-03-31")), domain.RuleMaturityViolation)
	if len(fs) != 1 || fs[0].Severity != domain.SeverityWarning || fs[0].ImpactMinor != 369863 || fs[0].InvoiceLineID != nil {
		t.Errorf("%+v", fs)
	}
}

func TestMaturityLocalTimezoneNoOffByOne(t *testing.T) {
	in := matInput("2026-03-31")
	loc := time.FixedZone("x", 3*3600)
	d := time.Date(2026, 3, 31, 0, 0, 0, 0, loc)
	in.Invoice.DueDate = &d
	in.Invoice.IssueDate = time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	fs := only(t, Run(in), domain.RuleMaturityViolation)
	if len(fs) != 1 || fs[0].ImpactMinor != 369863 {
		t.Errorf("%+v", fs)
	}
}

func TestMaturityOK(t *testing.T) {
	if fs := only(t, Run(matInput("2026-04-30")), domain.RuleMaturityViolation); len(fs) != 0 {
		t.Errorf("%+v", fs)
	}
}

func TestMaturityNoDueDate(t *testing.T) {
	if fs := only(t, Run(matInput("")), domain.RuleMaturityViolation); len(fs) != 0 {
		t.Errorf("%+v", fs)
	}
}
