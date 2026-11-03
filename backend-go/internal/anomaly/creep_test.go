package anomaly

import (
	"testing"
	"time"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func creepInput(net string, hist ...string) Input {
	lc := baseCtx("100", "0")
	lc.ContractItem = nil
	lc.Line.LineExtension = dec(net).Mul(dec("100"))
	lc.Line.UnitPrice = dec(net)
	for i, h := range hist {
		lc.PriceHistory = append(lc.PriceHistory, domain.PricePoint{IssueDate: date("2026-01-01").Add(time.Duration(i) * 24 * time.Hour), NetUnitPrice: dec(h)})
	}
	in := baseInput(lc)
	in.Contract = nil
	return in
}

func TestCreepEMA(t *testing.T) {
	fs := only(t, Run(creepInput("20", "10", "12", "14")), domain.RulePriceCreep)
	if len(fs) != 1 || fs[0].Evidence["ema"] != "11.55102" {
		t.Fatalf("%+v", fs)
	}
}

func TestCreepTriggers(t *testing.T) {
	fs := only(t, Run(creepInput("11.01", "10", "10", "10")), domain.RulePriceCreep)
	if len(fs) != 1 || fs[0].Severity != domain.SeverityWarning || fs[0].ImpactMinor != 10100 {
		t.Fatalf("%+v", fs)
	}
}

func TestCreepAtThreshold(t *testing.T) {
	if fs := only(t, Run(creepInput("11.00", "10", "10", "10")), domain.RulePriceCreep); len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}

func TestCreepTooFewPoints(t *testing.T) {
	if fs := only(t, Run(creepInput("20", "10", "10")), domain.RulePriceCreep); len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}

func TestCreepSkipsContracted(t *testing.T) {
	in := creepInput("20", "10", "10", "10")
	in.Lines[0].ContractItem = baseItem()
	if fs := only(t, Run(in), domain.RulePriceCreep); len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}
