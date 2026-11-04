package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// FXSource returns the TCMB bulletin of a day; published=false means none was issued (weekend, holiday).
// Implemented by tcmb.Client.
type FXSource interface {
	Day(ctx context.Context, day time.Time) ([]domain.FXQuote, bool, error)
}

const fxLookback = 10 // longest Turkish holiday run (Kurban Bayramı + bridge days) stays inside this

// Whole-lookup budget, well under the API's 60s WriteTimeout (a cold cache may need several TCMB requests).
const fxTimeout = 20 * time.Second

var trt = time.FixedZone("TRT", 3*60*60) // Türkiye has no DST since 2016

// fxFallback finds TCMB rates (D17) from the invoice currency to every contract item currency the invoice's own
// PricingExchangeRate does not cover: forex buying of the latest bulletin published before the issue date.
// It runs outside the audit transaction because it may call TCMB; any error means "no fallback" (FX_UNVERIFIED).
func (s *Service) fxFallback(ctx context.Context, tenantID, invoiceID uuid.UUID) (map[string]domain.ExchangeRate, error) {
	if s.fx == nil {
		return nil, nil
	}
	var inv domain.Invoice
	need := map[string]bool{}
	err := s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
		var err error
		if inv, err = tx.LoadInvoice(ctx, invoiceID); err != nil {
			return err
		}
		_, items, err := tx.ActiveContract(ctx, inv.VendorID, inv.IssueDate)
		for _, it := range items {
			r := inv.ExchangeRate
			if it.Currency != inv.Currency && (r == nil || r.Source != inv.Currency || r.Target != it.Currency) {
				need[it.Currency] = true
			}
		}
		return err
	})
	if err != nil || len(need) == 0 {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, fxTimeout)
	defer cancel()
	today := time.Now().In(trt)
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	for i := 1; i <= fxLookback; i++ {
		day := inv.IssueDate.AddDate(0, 0, -i)
		var known, published bool
		var quotes []domain.FXQuote
		err := s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
			var err error
			known, published, quotes, err = tx.FXDay(ctx, day)
			return err
		})
		if err != nil {
			return nil, err
		}
		if !known {
			if quotes, published, err = s.fx.Day(ctx, day); err != nil {
				return nil, err
			}
			// today's bulletin appears at 15:30: a missing one is not a holiday yet, so it is not remembered
			if published || day.Before(todayDate) {
				if err := s.store.InTenant(ctx, tenantID, func(tx *repository.Tx) error {
					return tx.SaveFXDay(ctx, day, published, quotes)
				}); err != nil {
					return nil, err
				}
			}
		}
		if published {
			return crossRates(inv.Currency, need, quotes, day), nil
		}
	}
	return nil, fmt.Errorf("no TCMB bulletin in the %d days before %s", fxLookback, inv.IssueDate.Format("2006-01-02"))
}

// crossRates converts from -> each target through TRY: (TRY per from) / (TRY per target). Unquoted pairs are left out.
func crossRates(from string, targets map[string]bool, quotes []domain.FXQuote, day time.Time) map[string]domain.ExchangeRate {
	perTRY := map[string]decimal.Decimal{"TRY": decimal.NewFromInt(1)}
	for _, q := range quotes {
		perTRY[q.Currency] = q.ForexBuying.Div(decimal.NewFromInt(q.Unit))
	}
	out := map[string]domain.ExchangeRate{}
	src, ok := perTRY[from]
	if !ok {
		return out
	}
	for to := range targets {
		if dst, ok := perTRY[to]; ok {
			out[to] = domain.ExchangeRate{Source: from, Target: to, Rate: src.Div(dst), TCMBDate: day}
		}
	}
	return out
}
