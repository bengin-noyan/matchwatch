// Package audit, denetimi tek bir tenant transaction'ında baştan sona yürütür: yükle, eşleştir, kuralları çalıştır,
// kaydet.
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
	emb   Embedder // nil ise öneri üretilmez (Faz 1'deki gibi)
	fx    FXSource // nil ise TCMB kuru kullanılmaz (Faz 1'deki gibi: FX_UNVERIFIED)
}

func New(store *repository.Store, emb Embedder, fx FXSource) *Service {
	return &Service{store: store, emb: emb, fx: fx}
}

// AuditInvoice kaç kez çalışırsa çalışsın aynı sonucu verir: OPEN bulgular ve dağıtımlar yeniden kurulur, DISPUTED ve
// APPROVED_EXCEPTION olanlar korunur.
func (s *Service) AuditInvoice(ctx context.Context, tenantID, invoiceID uuid.UUID) (domain.AuditStatus, error) {
	suggestions, err := s.suggest(ctx, tenantID, invoiceID)
	if err != nil {
		log.Printf("audit %s: semantic suggestions skipped: %v", invoiceID, err) // öneriler alınamazsa denetimin kendisi yine de çalışmalı (D16)
	}
	fxFallback, err := s.fxFallback(ctx, tenantID, invoiceID)
	if err != nil {
		log.Printf("audit %s: TCMB rates skipped: %v", invoiceID, err) // D17: kur bulunamazsa fiyat kuralı FX_UNVERIFIED yazar
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
				if lc.MatchMethod == anomaly.MatchAlias { // kodsuz ama eşleşmesi onaylanmış bir satır, irsaliyeleri sözleşme kaleminin kodlarıyla arar
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
			lc.Delivery = matcher.Allocate(delivery, cands) // cands ortak; böylece iki satır aynı miktarı iki kez kullanamaz
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
		// yalnızca yeni bulgular ve korunan DISPUTED bulgular sayılır; APPROVED_EXCEPTION sayılmaz
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

// Resolve, verilen her bulguya bir insan kararı yazar; ya hepsi olur ya hiçbiri. Etkilenen faturaların denetim durumunu
// aynı transaction'da günceller. Bilinmeyen bir id (ya da başka bir tenant'ın id'si) bütün çağrıyı
// repository.ErrNotFound ile düşürür. Kaç ayrı bulguya karar verildiğini döner.
// Yeniden denetim yapmaz, çünkü o zaman OPEN bulgular yeni id'lerle yeniden kurulurdu.
func (s *Service) Resolve(ctx context.Context, tenantID uuid.UUID, findingIDs []uuid.UUID, res domain.Resolution, note string, by *uuid.UUID) (int, error) {
	slices.SortFunc(findingIDs, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	findingIDs = slices.Compact(findingIDs) // aynı id iki kez gelirse geçmişe iki kez yazılırdı
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
