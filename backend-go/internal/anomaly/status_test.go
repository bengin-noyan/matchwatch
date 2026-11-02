package anomaly

import (
	"testing"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func TestStatus(t *testing.T) {
	f := func(s domain.Severity) domain.Finding { return domain.Finding{Severity: s} }
	cases := []struct {
		in   []domain.Finding
		want domain.AuditStatus
	}{
		{nil, domain.AuditGreen},
		{[]domain.Finding{f(domain.SeverityInfo)}, domain.AuditGreen},
		{[]domain.Finding{f(domain.SeverityWarning), f(domain.SeverityInfo)}, domain.AuditYellow},
		{[]domain.Finding{f(domain.SeverityWarning), f(domain.SeverityCritical)}, domain.AuditRed},
	}
	for i, c := range cases {
		if got := Status(c.in); got != c.want {
			t.Errorf("case %d: %s want %s", i, got, c.want)
		}
	}
}
