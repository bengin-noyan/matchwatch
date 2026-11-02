package anomaly

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/matcher"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func baseItem() *domain.ContractItem {
	return &domain.ContractItem{ItemCode: "A1", UOM: "ADET", Currency: "TRY", UnitPrice: dec("1.250000"),
		SourcePage: 3, SourceClauseText: "clause", RequiresDelivery: true}
}

func baseCtx(qty, ext string) LineContext {
	return LineContext{
		Line: domain.InvoiceLine{ID: uuid.New(), LineID: "1", ItemCode: "A1", UOM: "ADET",
			Qty: dec(qty), UnitPrice: dec(ext).Div(dec(qty)), LineExtension: dec(ext)},
		ContractItem: baseItem(),
		Delivery:     matcher.DeliveryResult{AvailableTotal: dec(qty), CandidateCount: 1},
	}
}

func baseInput(lc LineContext) Input {
	return Input{
		Invoice:  domain.Invoice{Currency: "TRY", IssueDate: date("2026-03-01")},
		Contract: &domain.Contract{Number: "C-1", PaymentTermDays: 60, Currency: "TRY", QtyTolerancePct: dec("2.00")},
		Lines:    []LineContext{lc},
	}
}

func only(t *testing.T, fs []domain.Finding, rule domain.RuleCode) []domain.Finding {
	t.Helper()
	var out []domain.Finding
	for _, f := range fs {
		if f.RuleCode == rule {
			out = append(out, f)
		}
	}
	return out
}
