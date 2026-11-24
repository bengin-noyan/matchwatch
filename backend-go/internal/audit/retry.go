package audit

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// RunRetries, ctx bitene kadar hemen ve sonra her 5 dakikada bir RetryPending çalıştırır.
func (s *Service) RunRetries(ctx context.Context) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for ctx.Err() == nil {
		if err := s.RetryPending(ctx); err != nil {
			log.Printf("audit retry: %v", err)
		}
		select {
		case <-ctx.Done():
		case <-tick.C:
		}
	}
}

// RetryPending, fatura kaydedildikten sonra denetimi başarısız olan (PENDING kalan) faturaları bütün tenant'larda yeniden
// denetler. Bir faturanın ya da tenant'ın hatası log'a yazılır, kalanları durdurmaz.
// shortcut: her turda bütün tenant'lar sırayla sorgulanır; tenant sayısı binleri bulunca tek sorguya çevir.
func (s *Service) RetryPending(ctx context.Context) error {
	tenants, err := s.store.AuditTenants(ctx)
	if err != nil {
		return err
	}
	for _, tid := range tenants {
		var ids []uuid.UUID
		err := s.store.InTenant(ctx, tid, func(tx *repository.Tx) (err error) {
			ids, err = tx.StuckInvoices(ctx)
			return
		})
		if err != nil {
			log.Printf("audit retry: tenant %s: %v", tid, err)
		}
		for _, id := range ids {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := s.retryOne(ctx, tid, id); err != nil {
				log.Printf("audit retry: invoice %s: %v", id, err)
			}
		}
	}
	return nil
}

// retryOne panic'i hataya çevirir: burada net/http'nin koruması yok, panic bütün API'yi düşürürdü.
func (s *Service) retryOne(ctx context.Context, tid, id uuid.UUID) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	_, err = s.AuditInvoice(ctx, tid, id)
	return err
}
