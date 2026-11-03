package anomaly

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func whInput(code, pct, amount string) Input {
	lc := baseCtx("1", "1000")
	lc.ContractItem.WithholdingCode = "601"
	lc.Line.VATRate, lc.Line.WithholdingCode = dec("20"), code
	lc.Line.WithholdingPercent, lc.Line.WithholdingAmount = dec(pct), dec(amount)
	in := baseInput(lc)
	in.WithholdingPercents = map[string]decimal.Decimal{"601": dec("40")}
	return in
}

func TestWithholdingOK(t *testing.T) {
	if fs := only(t, Run(whInput("601", "40", "80")), domain.RuleWithholdingError); len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}

func TestWithholdingWrongPercent(t *testing.T) {
	fs := only(t, Run(whInput("601", "20", "40")), domain.RuleWithholdingError)
	if len(fs) != 1 || fs[0].Severity != domain.SeverityCritical || fs[0].ImpactMinor != 4000 {
		t.Fatalf("%+v", fs)
	}
}

func TestWithholdingMissing(t *testing.T) {
	fs := only(t, Run(whInput("", "0", "0")), domain.RuleWithholdingError)
	if len(fs) != 1 || fs[0].Severity != domain.SeverityCritical || fs[0].ImpactMinor != 8000 {
		t.Fatalf("%+v", fs)
	}
}

func TestWithholdingUnknownCode(t *testing.T) {
	in := whInput("999", "10", "20")
	in.Lines[0].ContractItem.WithholdingCode = ""
	fs := only(t, Run(in), domain.RuleWithholdingError)
	if len(fs) != 1 || fs[0].Severity != domain.SeverityCritical {
		t.Fatalf("%+v", fs)
	}
}
