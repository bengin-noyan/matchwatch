package audit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

type fakeTCMB struct {
	calls []string
	days  map[string][]domain.FXQuote // yayımlanmış bültenler
	fail  map[string]bool             // geçici hatalar
}

func (f *fakeTCMB) Day(_ context.Context, day time.Time) ([]domain.FXQuote, bool, error) {
	d := day.Format("2006-01-02")
	f.calls = append(f.calls, d)
	if f.fail[d] {
		return nil, false, errors.New("HTTP 500")
	}
	q, ok := f.days[d]
	return q, ok, nil
}

// clearFX, sabit test günlerini global kur tablolarından testten önce ve sonra siler; bu tablolar her test
// çalıştırmasında ortak.
func clearFX(t *testing.T) {
	t.Helper()
	del := func() {
		ctx := context.Background()
		owner, err := pgx.Connect(ctx, os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		defer owner.Close(ctx)
		for _, q := range []string{`DELETE FROM fx_rates WHERE rate_date BETWEEN '2026-02-15' AND '2026-02-28'`,
			`DELETE FROM fx_days WHERE rate_date BETWEEN '2026-02-15' AND '2026-02-28'`} {
			if _, err := owner.Exec(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
	}
	del()
	t.Cleanup(del)
}

// usdContract, temel sözleşmeyi USD fiyatlarla günceller: M8 cıvata 0,03 USD, bakım 25 USD.
func (e *env) usdContract(t *testing.T) {
	e.exec(t, `UPDATE contract_items SET currency='USD', unit_price = CASE WHEN item_code='CV-M8-20' THEN 0.03 ELSE 25 END`)
}

func TestTCMBFallbackWeekendChainAndCache(t *testing.T) {
	e := setup(t)
	clearFX(t)
	e.usdContract(t)
	// invoice_basic'in tarihi 1 Mart 2026 Pazar: cumartesi bülten yok, cumanınki kullanılır
	f := &fakeTCMB{days: map[string][]domain.FXQuote{"2026-02-27": {
		{Currency: "USD", Unit: 1, ForexBuying: decimal.RequireFromString("40.0000")},
		{Currency: "JPY", Unit: 100, ForexBuying: decimal.RequireFromString("26.0000")},
	}}}
	e.svc = New(e.store, nil, f)
	inv := e.ingest(t, false)
	e.audit(t, inv)
	// 1. satır: 1,30 TRY / 40 = 0,0325 USD, sözleşme 0,03 → 2,50 USD fark; 2. satır: 1000 / 40 = 25 USD, fark yok
	rows := e.findings(t, inv)
	p := ruleRow(rows, domain.RulePriceMismatch, "1")
	if p == nil || p.impact != 250 || p.evidence["fx_source"] != "TCMB" || p.evidence["fx_rate_date"] != "2026-02-27" ||
		p.evidence["fx_rate"] != "0.025" || ruleRow(rows, domain.RuleFXUnverified, "1") != nil {
		t.Fatalf("findings %v / %+v", sig(rows), p)
	}
	if fmt.Sprint(f.calls) != "[2026-02-28 2026-02-27]" {
		t.Fatalf("calls %v", f.calls)
	}
	e.audit(t, inv) // iki gün de artık önbellekte, tatil günü dahil
	if len(f.calls) != 2 {
		t.Fatalf("re-audit fetched again: %v", f.calls)
	}
	// uygulama rolü bülten ekleyebilir ama var olanı asla değiştiremez
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE fx_rates SET forex_buying=1 WHERE rate_date='2026-02-27'`)
		return err
	})
	if err == nil {
		t.Fatal("app role updated fx_rates")
	}
}

func TestFXRateDeviationAgainstTCMB(t *testing.T) {
	e := setup(t)
	clearFX(t)
	f := &fakeTCMB{days: map[string][]domain.FXQuote{"2026-02-27": {
		{Currency: "USD", Unit: 1, ForexBuying: decimal.RequireFromString("40.0000")}}}}
	e.svc = New(e.store, nil, f)
	inv := e.ingest(t, false)
	// TRY sözleşmesi için kur gerekmez; ama faturanın kendi USD→TRY 42 kuru yine de kontrol edilir (D18)
	e.exec(t, `UPDATE invoices SET currency='USD', fx_source='USD', fx_target='TRY', fx_rate=42 WHERE id=$1`, inv)
	var payable int64
	if err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT payable_minor FROM invoices WHERE id=$1`, inv).Scan(&payable)
	}); err != nil {
		t.Fatal(err)
	}
	e.audit(t, inv)
	rows := e.findings(t, inv)
	d := ruleRow(rows, domain.RuleFXRateDeviation, "")
	if d == nil || d.impact != payable*2 || d.evidence["deviation_pct"] != "5.00" || d.evidence["tcmb_rate_date"] != "2026-02-27" {
		t.Fatalf("findings %v / %+v", sig(rows), d)
	}
	if p := ruleRow(rows, domain.RulePriceMismatch, "1"); p == nil || p.evidence["fx_source"] != "TCMB" || p.evidence["fx_rate"] != "40" {
		t.Fatalf("price must use TCMB, not the deviating rate: %v / %+v", sig(rows), p)
	}
}

func TestTCMBTransientErrorIsNotCached(t *testing.T) {
	e := setup(t)
	clearFX(t)
	e.usdContract(t)
	f := &fakeTCMB{fail: map[string]bool{"2026-02-28": true}}
	e.svc = New(e.store, nil, f)
	inv := e.ingest(t, false)
	e.audit(t, inv)
	rows := e.findings(t, inv)
	if ruleRow(rows, domain.RuleFXUnverified, "1") == nil || ruleRow(rows, domain.RulePriceMismatch, "1") != nil {
		t.Fatalf("want FX_UNVERIFIED, got %v", sig(rows))
	}
	if n := e.count(t, `SELECT count(*) FROM fx_days WHERE rate_date='2026-02-28'`); n != 0 {
		t.Fatalf("transient error cached (%d rows)", n)
	}
}
