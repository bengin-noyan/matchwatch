package anomaly

import (
	"testing"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func qtyFindings(t *testing.T, in Input) []domain.Finding {
	return only(t, Run(in), domain.RuleQtyExceedsDelivery)
}

func TestQtyWithinTolerance(t *testing.T) {
	lc := baseCtx("969", "1211.25")
	lc.Delivery.AvailableTotal = dec("950")
	if fs := qtyFindings(t, baseInput(lc)); len(fs) != 0 {
		t.Errorf("%+v", fs)
	}
}

func TestQtyExceeds(t *testing.T) {
	lc := baseCtx("970", "1261")
	lc.Delivery.AvailableTotal = dec("950")
	fs := qtyFindings(t, baseInput(lc))
	if len(fs) != 1 || fs[0].Severity != domain.SeverityCritical || fs[0].ImpactMinor != 2600 || fs[0].ImpactCurrency != "TRY" {
		t.Fatalf("%+v", fs)
	}
	e := fs[0].Evidence
	if e["available_total"] != "950" || e["tolerance_pct"] != "2" || e["line_id"] != "1" {
		t.Errorf("%+v", e)
	}
	if _, ok := e["no_despatch_found"]; ok {
		t.Errorf("unexpected no_despatch_found")
	}
}

func TestQtyNoDespatchServiceLine(t *testing.T) {
	lc := baseCtx("10", "100")
	lc.ContractItem = nil
	lc.Delivery.CandidateCount = 0
	lc.Delivery.AvailableTotal = dec("0")
	if fs := qtyFindings(t, baseInput(lc)); len(fs) != 0 {
		t.Errorf("%+v", fs)
	}
}

func TestQtyNoDespatchContractedLine(t *testing.T) {
	lc := baseCtx("10", "12.5")
	lc.Delivery.CandidateCount = 0
	lc.Delivery.AvailableTotal = dec("0")
	fs := qtyFindings(t, baseInput(lc))
	if len(fs) != 1 || fs[0].Severity != domain.SeverityCritical || fs[0].Evidence["no_despatch_found"] != true {
		t.Errorf("%+v", fs)
	}
}

func TestQtyContractedServiceLine(t *testing.T) {
	lc := baseCtx("10", "12.5")
	lc.ContractItem.RequiresDelivery = false
	lc.Delivery.CandidateCount = 0
	lc.Delivery.AvailableTotal = dec("0")
	if fs := qtyFindings(t, baseInput(lc)); len(fs) != 0 {
		t.Errorf("%+v", fs)
	}
}
