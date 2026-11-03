// Package audit orchestrates load -> match -> rules -> save in one tenant transaction.
package audit

import (
	"context"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/anomaly"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/matcher"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

type Service struct{ store *repository.Store }

func New(store *repository.Store) *Service { return &Service{store: store} }

// AuditInvoice is idempotent: OPEN findings and allocations are rebuilt, DISPUTED/APPROVED_EXCEPTION ones are kept.
func (s *Service) AuditInvoice(ctx context.Context, tenantID, invoiceID uuid.UUID) (domain.AuditStatus, error) {
	var status domain.AuditStatus
	err := s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
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
		in := anomaly.Input{Invoice: inv, Contract: contract}
		var allocs []domain.Allocation
		matched := map[uuid.UUID]uuid.UUID{}
		for _, line := range inv.Lines {
			lc := anomaly.LineContext{Line: line, ContractItem: matcher.MatchContractItem(line, items)}
			if lc.ContractItem != nil {
				matched[line.ID] = lc.ContractItem.ID
			}
			lc.Delivery = matcher.Allocate(line, cands) // cands is shared, so lines cannot reuse the same quantity
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
