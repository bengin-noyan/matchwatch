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

// FXSource, bir günün TCMB bültenini getirir. published=false o gün bülten yayımlanmadığını gösterir (hafta sonu,
// tatil). tcmb.Client bunu uygular.
type FXSource interface {
	Day(ctx context.Context, day time.Time) ([]domain.FXQuote, bool, error)
}

const fxLookback = 10 // en uzun tatil bile (Kurban Bayramı ve köprü günleri) bu süreye sığar

// Bütün arama için süre sınırı. API'nin 60 saniyelik WriteTimeout'unun epey altında; önbellek boşsa TCMB'ye birkaç
// istek atmak gerekebilir.
const fxTimeout = 20 * time.Second

var trt = time.FixedZone("TRT", 3*60*60) // Türkiye 2016'dan beri yaz saati uygulamıyor

// fxFallback, fatura para biriminden TCMB kurlarını bulur (D17). Faturanın kendi kurunun karşılamadığı her sözleşme
// para birimi için, ayrıca faturanın kendi kur çifti için (D18), düzenleme tarihinden önce yayımlanan son bültenin
// döviz alış kurunu kullanır.
// TCMB'yi çağırabileceği için denetim transaction'ının dışında çalışır. Bir hata olursa TCMB kuru kullanılmaz ve fiyat
// kuralı FX_UNVERIFIED yazar.
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
		if r := inv.ExchangeRate; r != nil && r.Source == inv.Currency && r.Target != r.Source {
			need[r.Target] = true // D18: sözleşme olsun olmasın, faturanın kendi kurunu TCMB ile karşılaştırıyoruz
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
			// bugünün bülteni 15:30'da çıkar; henüz yoksa tatil olduğunu bilemeyiz, o yüzden "bülten yok" diye kaydetmiyoruz
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

// crossRates, kaynaktan her hedefe TRY üzerinden çapraz kur hesaplar: (kaynağın TRY karşılığı) / (hedefin TRY
// karşılığı). Bültende olmayan çiftler sonuca girmez.
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
