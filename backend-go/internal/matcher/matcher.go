// Package matcher is pure: no DB, no network.
package matcher

import (
	"sort"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

type DeliveryResult struct {
	Allocations    []domain.Allocation
	AvailableTotal decimal.Decimal
	CandidateCount int
}

// MatchContractItem matches by ItemCode first, then Barcode. Empty codes never match. UOM is not checked here.
func MatchContractItem(line domain.InvoiceLine, items []domain.ContractItem) *domain.ContractItem {
	if line.ItemCode != "" {
		for i := range items {
			if items[i].ItemCode == line.ItemCode {
				return &items[i]
			}
		}
	}
	if line.Barcode != "" {
		for i := range items {
			if items[i].Barcode == line.Barcode {
				return &items[i]
			}
		}
	}
	return nil
}

// Allocate fills the line oldest-shipment-first from cands with the same code/barcode and UOM,
// and decrements cands[i].Available in place so later lines cannot reuse the quantity.
func Allocate(line domain.InvoiceLine, cands []domain.DespatchCandidate) DeliveryResult {
	res := DeliveryResult{AvailableTotal: decimal.Zero}
	var idx []int
	for i, c := range cands {
		codeHit := line.ItemCode != "" && c.ItemCode == line.ItemCode
		barHit := line.Barcode != "" && c.Barcode == line.Barcode
		if (codeHit || barHit) && c.UOM == line.UOM {
			idx = append(idx, i)
			res.AvailableTotal = res.AvailableTotal.Add(c.Available)
		}
	}
	res.CandidateCount = len(idx)
	sort.SliceStable(idx, func(a, b int) bool {
		ca, cb := cands[idx[a]], cands[idx[b]]
		if !ca.ShipmentDate.Equal(cb.ShipmentDate) {
			return ca.ShipmentDate.Before(cb.ShipmentDate)
		}
		return ca.LineID.String() < cb.LineID.String()
	})
	remaining := line.Qty
	for _, i := range idx {
		if !remaining.IsPositive() {
			break
		}
		if !cands[i].Available.IsPositive() {
			continue
		}
		take := decimal.Min(remaining, cands[i].Available)
		res.Allocations = append(res.Allocations, domain.Allocation{InvoiceLineID: line.ID, DespatchLineID: cands[i].LineID, Qty: take})
		cands[i].Available = cands[i].Available.Sub(take)
		remaining = remaining.Sub(take)
	}
	return res
}

var suggestionFloor = decimal.RequireFromString("0.50")

const maxSuggestions = 3

// MatchAlias resolves a human-confirmed alias (D16). An alias whose item is not in the active contract is ignored.
func MatchAlias(line domain.InvoiceLine, items []domain.ContractItem, aliases map[domain.AliasKey]uuid.UUID) *domain.ContractItem {
	id, ok := aliases[domain.AliasKey{NameKey: domain.NameKey(line.Name), UOM: line.UOM}]
	if !ok {
		return nil
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

// PickSuggestions keeps candidates (already ordered best first) at or above the floor, at most three.
func PickSuggestions(cands []domain.Suggestion) []domain.Suggestion {
	var out []domain.Suggestion
	for _, c := range cands {
		if len(out) == maxSuggestions {
			break
		}
		if c.Similarity.GreaterThanOrEqual(suggestionFloor) {
			out = append(out, c)
		}
	}
	return out
}
