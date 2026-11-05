// Package audit orchestrates load -> match -> rules -> save in one tenant transaction.
package audit

import (
	"bytes"
	"context"
	"log"
	"slices"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/anomaly"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/matcher"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

type Service struct {
	store *repository.Store
	emb   Embedder // nil = no semantic suggestions (Faz 1 behaviour)
	fx    FXSource // nil = no TCMB fallback (Faz 1 behaviour: FX_UNVERIFIED)
}

func New(store *repository.Store, emb Embedder, fx FXSource) *Service {
	return &Service{store: store, emb: emb, fx: fx}
}

// AuditInvoice is idempotent: OPEN findings and allocations are rebuilt, DISPUTED/APPROVED_EXCEPTION ones are kept.
func (s *Service) AuditInvoice(ctx context.Context, tenantID, invoiceID uuid.UUID) (domain.AuditStatus, error) {
	suggestions, err := s.suggest(ctx, tenantID, invoiceID)
	if err != nil {
		log.Printf("audit %s: semantic suggestions skipped: %v", invoiceID, err) // the audit itself must not fail (D16)
	}
	fxFallback, err := s.fxFallback(ctx, tenantID, invoiceID)
	if err != nil {
		log.Printf("audit %s: TCMB rates skipped: %v", invoiceID, err) // D17: the price rule falls back to FX_UNVERIFIED
	}
	var status domain.AuditStatus
	err = s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
		kept, err := tx.ResetAudit(ctx, invoiceID)
		if err != nil {
			return err
		}
		inv, err := tx.LoadInvoice(ctx, invoiceID)
		if err != nil {
			return err
		}
		contract, items, err := tx.ActiveContract(ctx, inv.VendorID, inv.IssueDate)
		if err != nil {
			return err
		}
		cands, err := tx.DespatchCandidates(ctx, inv)
		if err != nil {
			return err
		}
		var aliases map[domain.AliasKey]uuid.UUID
		if contract != nil {
			if aliases, err = tx.Aliases(ctx, inv.VendorID); err != nil {
				return err
			}
		}
		in := anomaly.Input{Invoice: inv, Contract: contract, FXFallback: fxFallback}
		var allocs []domain.Allocation
		matched := map[uuid.UUID]uuid.UUID{}
		for _, line := range inv.Lines {
			lc := anomaly.LineContext{Line: line, ContractItem: matcher.MatchContractItem(line, items)}
			if lc.ContractItem == nil {
				if lc.ContractItem = matcher.MatchAlias(line, items, aliases); lc.ContractItem != nil {
					lc.MatchMethod = anomaly.MatchAlias
				}
			}
			delivery := line
			if lc.ContractItem != nil {
				matched[line.ID] = lc.ContractItem.ID
				if lc.MatchMethod == anomaly.MatchAlias { // a code-less aliased line draws on despatches by the contract item's codes
					if delivery.ItemCode == "" {
						delivery.ItemCode = lc.ContractItem.ItemCode
					}
					if delivery.Barcode == "" {
						delivery.Barcode = lc.ContractItem.Barcode
					}
				}
			} else {
				lc.Suggestions = suggestions[line.ID]
			}
			lc.Delivery = matcher.Allocate(delivery, cands) // cands is shared, so lines cannot reuse the same quantity
			allocs = append(allocs, lc.Delivery.Allocations...)
			if lc.PriceHistory, err = tx.PriceHistory(ctx, inv, line); err != nil {
				return err
			}
			in.Lines = append(in.Lines, lc)
		}
		if in.Duplicates, err = tx.DuplicateCandidates(ctx, inv); err != nil {
			return err
		}
		if in.WithholdingPercents, err = tx.WithholdingPercents(ctx); err != nil {
			return err
		}
		if in.FinancingRateAnnual, err = tx.FinancingRate(ctx); err != nil {
			return err
		}

		var fresh []domain.Finding
		for _, f := range anomaly.Run(in) {
			if !isKept(kept, f) {
				fresh = append(fresh, f)
			}
		}
		// only new findings and DISPUTED kept ones count; APPROVED_EXCEPTION does not
		counted := append([]domain.Finding(nil), fresh...)
		for _, k := range kept {
			if k.Resolution == domain.ResolutionDisputed {
				counted = append(counted, domain.Finding{Severity: k.Severity})
			}
		}
		status = anomaly.Status(counted)
		return tx.SaveAudit(ctx, invoiceID, fresh, allocs, matched, status)
	})
	return status, err
}

// Resolve records one human decision on every given finding, all or nothing, and refreshes the touched
// invoices' audit status in the same transaction. Unknown ids (or another tenant's) fail the whole call with
// repository.ErrNotFound. It returns how many distinct findings it decided on.
// It does not re-audit: OPEN findings would be rebuilt with new ids.
func (s *Service) Resolve(ctx context.Context, tenantID uuid.UUID, findingIDs []uuid.UUID, res domain.Resolution, note string, by *uuid.UUID) (int, error) {
	slices.SortFunc(findingIDs, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	findingIDs = slices.Compact(findingIDs) // a repeated id would be recorded twice in the history
	return len(findingIDs), s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
		invoices, err := tx.LockFindingInvoices(ctx, findingIDs)
		if err != nil {
			return err
		}
		for _, id := range findingIDs {
			if _, err := tx.UpdateFinding(ctx, id, res, note, by); err != nil {
				return err
			}
		}
		for _, inv := range invoices {
			sevs, err := tx.CountedSeverities(ctx, inv)
			if err != nil {
				return err
			}
			fs := make([]domain.Finding, len(sevs))
			for i, sev := range sevs {
				fs[i].Severity = sev
			}
			if err := tx.SetAuditStatus(ctx, inv, anomaly.Status(fs)); err != nil {
				return err
			}
		}
		return nil
	})
}

func isKept(kept []repository.KeptFinding, f domain.Finding) bool {
	for _, k := range kept {
		if k.RuleCode != f.RuleCode {
			continue
		}
		if k.InvoiceLineID == nil && f.InvoiceLineID == nil || k.InvoiceLineID != nil && f.InvoiceLineID != nil && *k.InvoiceLineID == *f.InvoiceLineID {
			return true
		}
	}
	return false
}
