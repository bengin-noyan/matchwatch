package matcher

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func day(m time.Month, dd int) time.Time { return time.Date(2026, m, dd, 0, 0, 0, 0, time.UTC) }

func cand(code, uom, avail string, date time.Time) domain.DespatchCandidate {
	return domain.DespatchCandidate{LineID: uuid.New(), ItemCode: code, UOM: uom, Available: d(avail), ShipmentDate: date}
}

func TestMatchByItemCode(t *testing.T) {
	items := []domain.ContractItem{{ItemCode: "A"}, {ItemCode: "B", Name: "hit"}}
	got := MatchContractItem(domain.InvoiceLine{ItemCode: "B"}, items)
	if got == nil || got.Name != "hit" {
		t.Fatalf("got %+v", got)
	}
}

func TestMatchByBarcodeWhenCodeDiffers(t *testing.T) {
	items := []domain.ContractItem{{ItemCode: "A", Barcode: "123", Name: "hit"}}
	got := MatchContractItem(domain.InvoiceLine{ItemCode: "X", Barcode: "123"}, items)
	if got == nil || got.Name != "hit" {
		t.Fatalf("got %+v", got)
	}
}

func TestNoMatch(t *testing.T) {
	items := []domain.ContractItem{{ItemCode: "A", Barcode: "1"}, {}}
	if got := MatchContractItem(domain.InvoiceLine{ItemCode: "X", Barcode: "9"}, items); got != nil {
		t.Fatalf("got %+v", got)
	}
	if got := MatchContractItem(domain.InvoiceLine{}, items); got != nil {
		t.Fatalf("empty codes must not match: %+v", got)
	}
}

func TestAllocateOldestFirst(t *testing.T) {
	newer := cand("A", "C62", "600", day(3, 1))
	older := cand("A", "C62", "600", day(2, 20))
	cands := []domain.DespatchCandidate{newer, older}
	line := domain.InvoiceLine{ID: uuid.New(), ItemCode: "A", UOM: "C62", Qty: d("1000")}
	r := Allocate(line, cands)
	if len(r.Allocations) != 2 ||
		r.Allocations[0].DespatchLineID != older.LineID || !r.Allocations[0].Qty.Equal(d("600")) ||
		r.Allocations[1].DespatchLineID != newer.LineID || !r.Allocations[1].Qty.Equal(d("400")) {
		t.Fatalf("allocs %+v", r.Allocations)
	}
	if !r.AvailableTotal.Equal(d("1200")) || r.CandidateCount != 2 {
		t.Fatalf("total %s count %d", r.AvailableTotal, r.CandidateCount)
	}
	if r.Allocations[0].InvoiceLineID != line.ID {
		t.Fatal("invoice line id not set")
	}
	if !cands[0].Available.Equal(d("200")) || !cands[1].Available.IsZero() {
		t.Fatalf("slice not updated in place: %s %s", cands[0].Available, cands[1].Available)
	}
	if cands[0].LineID != newer.LineID {
		t.Fatal("caller slice reordered")
	}
}

func TestAllocateSkipsOtherUOM(t *testing.T) {
	cands := []domain.DespatchCandidate{cand("A", "KGM", "500", day(2, 1))}
	r := Allocate(domain.InvoiceLine{ItemCode: "A", UOM: "C62", Qty: d("10")}, cands)
	if len(r.Allocations) != 0 || r.CandidateCount != 0 || !r.AvailableTotal.IsZero() {
		t.Fatalf("%+v", r)
	}
}

func TestAllocateConsumesAcrossLines(t *testing.T) {
	cands := []domain.DespatchCandidate{cand("A", "C62", "1000", day(2, 1))}
	line := domain.InvoiceLine{ItemCode: "A", UOM: "C62", Qty: d("700")}
	Allocate(line, cands)
	r := Allocate(line, cands)
	if !r.AvailableTotal.Equal(d("300")) || len(r.Allocations) != 1 || !r.Allocations[0].Qty.Equal(d("300")) {
		t.Fatalf("%+v", r)
	}
}

func TestAllocateNoCandidates(t *testing.T) {
	r := Allocate(domain.InvoiceLine{ItemCode: "A", UOM: "C62", Qty: d("1")}, nil)
	if r.CandidateCount != 0 || !r.AvailableTotal.IsZero() || len(r.Allocations) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestAllocateEmptyCodeNeverMatchesAndZeroSkipped(t *testing.T) {
	zero := cand("A", "C62", "0", day(2, 1))
	empty := cand("", "C62", "50", day(2, 2))
	cands := []domain.DespatchCandidate{zero, empty}
	r := Allocate(domain.InvoiceLine{ItemCode: "A", UOM: "C62", Qty: d("10")}, cands)
	if r.CandidateCount != 1 || len(r.Allocations) != 0 || !r.AvailableTotal.IsZero() {
		t.Fatalf("%+v", r)
	}
	r = Allocate(domain.InvoiceLine{UOM: "C62", Qty: d("10")}, cands)
	if r.CandidateCount != 0 {
		t.Fatalf("empty codes matched: %+v", r)
	}
}

func TestMatchAlias(t *testing.T) {
	hit := uuid.New()
	items := []domain.ContractItem{{ID: uuid.New()}, {ID: hit, Name: "hit"}}
	aliases := map[domain.AliasKey]uuid.UUID{{NameKey: "aycicek yagi 5lt", UOM: "C62"}: hit}
	if got := MatchAlias(domain.InvoiceLine{Name: "AYÇİÇEK YAĞI 5LT", UOM: "C62"}, items, aliases); got == nil || got.Name != "hit" {
		t.Fatalf("got %+v", got)
	}
	if got := MatchAlias(domain.InvoiceLine{Name: "AYÇİÇEK YAĞI 5LT", UOM: "KGM"}, items, aliases); got != nil {
		t.Fatalf("other uom matched: %+v", got)
	}
	// aktif sözleşmenin dışındaki bir kalemi gösteren eşleşme yok sayılır
	stale := map[domain.AliasKey]uuid.UUID{{NameKey: "aycicek yagi 5lt", UOM: "C62"}: uuid.New()}
	if got := MatchAlias(domain.InvoiceLine{Name: "AYÇİÇEK YAĞI 5LT", UOM: "C62"}, items, stale); got != nil {
		t.Fatalf("stale alias matched: %+v", got)
	}
}

func TestPickSuggestions(t *testing.T) {
	s := func(sim string) domain.Suggestion { return domain.Suggestion{ItemName: sim, Similarity: d(sim)} }
	got := PickSuggestions([]domain.Suggestion{s("0.9100"), s("0.4999"), s("0.5000"), s("0.6000"), s("0.5500")})
	if len(got) != 3 || got[0].ItemName != "0.9100" || got[1].ItemName != "0.5000" || got[2].ItemName != "0.6000" {
		t.Fatalf("got %+v", got)
	}
	if got := PickSuggestions([]domain.Suggestion{s("0.4999")}); len(got) != 0 {
		t.Fatalf("below floor kept: %+v", got)
	}
}
