package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/matcher"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// Embedder turns item names into vectors (raw JSON arrays) with the named model; implemented by aiclient.Client.
type Embedder interface {
	Embed(ctx context.Context, texts []string) (string, []json.RawMessage, error)
}

// shortcut: fixed timeout, move to config if CPU embedding of large invoices needs longer.
const embedTimeout = 60 * time.Second

const maxEmbedChars = 512 // ai-service limit per text

// embedText is what gets embedded: the normalized name (better recall in calibration), capped to the service limit.
func embedText(name string) string {
	r := []rune(domain.NameKey(name))
	return string(r[:min(len(r), maxEmbedChars)])
}

// suggest computes D16 suggestions for the invoice's lines that no code, barcode or alias matches.
// It runs outside the audit transaction because it calls the AI service; any error means "no suggestions".
func (s *Service) suggest(ctx context.Context, tenantID, invoiceID uuid.UUID) (map[uuid.UUID][]domain.Suggestion, error) {
	if s.emb == nil {
		return nil, nil
	}
	var lines []domain.InvoiceLine
	var contractID uuid.UUID
	err := s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
		inv, err := tx.LoadInvoice(ctx, invoiceID)
		if err != nil {
			return err
		}
		contract, items, err := tx.ActiveContract(ctx, inv.VendorID, inv.IssueDate)
		if err != nil || contract == nil {
			return err
		}
		aliases, err := tx.Aliases(ctx, inv.VendorID)
		if err != nil {
			return err
		}
		contractID = contract.ID
		for _, l := range inv.Lines {
			if matcher.MatchContractItem(l, items) == nil && matcher.MatchAlias(l, items, aliases) == nil && embedText(l.Name) != "" {
				lines = append(lines, l)
			}
		}
		return nil
	})
	if err != nil || len(lines) == 0 {
		return nil, err
	}
	ectx, cancel := context.WithTimeout(ctx, embedTimeout)
	defer cancel()
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = embedText(l.Name)
	}
	model, lineVecs, err := s.emb.Embed(ectx, texts)
	if err != nil {
		return nil, err
	}

	var need []domain.ContractItem
	err = s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
		all, err := tx.ItemsNeedingEmbedding(ctx, contractID, model)
		for _, it := range all {
			if embedText(it.Name) != "" {
				need = append(need, it)
			}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	var itemVecs []json.RawMessage
	if len(need) > 0 {
		texts = make([]string, len(need))
		for i, it := range need {
			texts[i] = embedText(it.Name)
		}
		var itemModel string
		if itemModel, itemVecs, err = s.emb.Embed(ectx, texts); err != nil {
			return nil, err
		}
		if itemModel != model {
			return nil, errors.New("embedding model changed between calls")
		}
	}

	out := map[uuid.UUID][]domain.Suggestion{}
	err = s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
		for i, it := range need {
			if err := tx.SetItemEmbedding(ctx, it.ID, model, itemVecs[i]); err != nil {
				return err
			}
		}
		for i, l := range lines {
			cands, err := tx.Suggestions(ctx, contractID, l.UOM, model, lineVecs[i])
			if err != nil {
				return err
			}
			if picked := matcher.PickSuggestions(cands); len(picked) > 0 {
				out[l.ID] = picked
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
