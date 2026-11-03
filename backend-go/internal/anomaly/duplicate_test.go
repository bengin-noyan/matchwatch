package anomaly

import (
	"testing"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func dupInput(num string, payable int64, name string, c domain.DuplicateCandidate) Input {
	in := baseInput(baseCtx("1", "1"))
	in.Invoice.Number, in.Invoice.PayableMinor = num, payable
	in.Invoice.Lines = []domain.InvoiceLine{{Name: name}}
	in.Duplicates = []domain.DuplicateCandidate{c}
	return in
}

func TestDuplicateSameNumber(t *testing.T) {
	c := domain.DuplicateCandidate{InvoiceID: uuid.New(), Number: "A1", PayableMinor: 5}
	fs := only(t, Run(dupInput("A1", 1180000, "x", c)), domain.RuleDuplicateSuspect)
	if len(fs) != 1 || fs[0].Severity != domain.SeverityCritical || fs[0].ImpactMinor != 1180000 ||
		fs[0].Evidence["match_reason"] != "same_number" {
		t.Fatalf("%+v", fs)
	}
}

func TestDuplicateSameAmountSimilarItems(t *testing.T) {
	c := domain.DuplicateCandidate{InvoiceID: uuid.New(), Number: "B", PayableMinor: 1180000,
		LineNames: []string{"CİVATA m8x20 paslanmaz"}}
	fs := only(t, Run(dupInput("A", 1180000, "Civata M8x20 Paslanmaz", c)), domain.RuleDuplicateSuspect)
	if len(fs) != 1 || fs[0].Severity != domain.SeverityCritical || fs[0].Evidence["match_reason"] != "same_amount_similar_items" {
		t.Fatalf("%+v", fs)
	}
}

func TestDuplicateSameAmountDifferentItems(t *testing.T) {
	c := domain.DuplicateCandidate{InvoiceID: uuid.New(), Number: "B", PayableMinor: 1180000,
		LineNames: []string{"Kağıt havlu"}}
	if fs := only(t, Run(dupInput("A", 1180000, "Civata", c)), domain.RuleDuplicateSuspect); len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}
