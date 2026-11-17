package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/migrate"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/ubl"
)

// setup, migration'ları uygular (owner), uygulama rolüyle bir Store ve test bitince silinen yeni bir tenant döner.
func setup(t *testing.T) (*Store, uuid.UUID) {
	t.Helper()
	ownerURL, appURL := os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"), os.Getenv("MATCHWATCH_TEST_DATABASE_URL")
	if ownerURL == "" || appURL == "" {
		t.Skip("MATCHWATCH_TEST_MIGRATE_URL / MATCHWATCH_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close(ctx) })
	if err := migrate.Apply(ctx, owner); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := NewStore(pool)
	vkn := uuid.NewString()[:8] + "00"
	tid, err := s.CreateTenant(ctx, "T", vkn, decimal.RequireFromString("0.4500"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		tx, err := owner.Begin(ctx)
		if err != nil {
			return
		}
		defer tx.Rollback(ctx)
		tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", tid.String())
		for _, tbl := range []string{"invoices", "despatch_advices", "contract_drafts", "contracts", "vendors"} {
			tx.Exec(ctx, "DELETE FROM "+tbl+" WHERE tenant_id=$1", tid)
		}
		tx.Exec(ctx, "DELETE FROM tenants WHERE id=$1", tid)
		tx.Commit(ctx)
	})
	return s, tid
}

func parseInvoice(t *testing.T) *ubl.Invoice {
	t.Helper()
	f, err := os.Open("../../testdata/invoice_basic.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	inv, err := ubl.ParseInvoice(f)
	if err != nil {
		t.Fatal(err)
	}
	inv.ETTN = uuid.NewString() // fixture'daki ETTN sabit; her çalıştırma birbirinden bağımsız olsun
	return inv
}

func parseDespatch(t *testing.T) *ubl.DespatchAdvice {
	t.Helper()
	f, err := os.Open("../../testdata/despatch_basic.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d, err := ubl.ParseDespatch(f)
	if err != nil {
		t.Fatal(err)
	}
	d.ETTN = uuid.NewString()
	return d
}

func parseReceipt(t *testing.T) *ubl.ReceiptAdvice {
	t.Helper()
	f, err := os.Open("../../testdata/receipt_basic.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := ubl.ParseReceipt(f)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestInsertInvoiceRoundTrip(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	inv := parseInvoice(t)
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		v, err := tx.UpsertVendor(ctx, inv.Supplier.VKN, inv.Supplier.Title)
		if err != nil {
			return err
		}
		id, err := tx.InsertInvoice(ctx, v, inv, "raw/inv.xml")
		if err != nil {
			return err
		}
		var ext, payable int64
		var price string
		if err := tx.QueryRow(ctx, `SELECT line_extension_minor, unit_price::text FROM invoice_lines WHERE invoice_id=$1 AND line_id='1'`, id).Scan(&ext, &price); err != nil {
			return err
		}
		if ext != 130000 || price != "1.400000" {
			t.Errorf("line 1: ext=%d price=%s", ext, price)
		}
		if err := tx.QueryRow(ctx, `SELECT payable_minor FROM invoices WHERE id=$1`, id).Scan(&payable); err != nil {
			return err
		}
		if payable != 268000 {
			t.Errorf("payable=%d", payable)
		}
		var refs int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM invoice_despatch_refs WHERE invoice_id=$1`, id).Scan(&refs); err != nil {
			return err
		}
		var rawPath string
		if err := tx.QueryRow(ctx, `SELECT raw_xml_path FROM invoices WHERE id=$1`, id).Scan(&rawPath); err != nil {
			return err
		}
		if rawPath != "raw/inv.xml" {
			t.Errorf("raw_xml_path=%q", rawPath)
		}
		if refs != 1 {
			t.Errorf("refs=%d", refs)
		}
		got, err := tx.InvoicesReferencingDespatch(ctx, v, "IRS2026000000001")
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0] != id {
			t.Errorf("InvoicesReferencingDespatch=%v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInsertInvoiceDuplicateETTN(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	inv := parseInvoice(t)
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		v, err := tx.UpsertVendor(ctx, inv.Supplier.VKN, inv.Supplier.Title)
		if err != nil {
			return err
		}
		if _, err := tx.InsertInvoice(ctx, v, inv, "a"); err != nil {
			return err
		}
		_, err = tx.InsertInvoice(ctx, v, inv, "b")
		return err
	})
	if !errors.Is(err, ErrDuplicateETTN) {
		t.Fatalf("want ErrDuplicateETTN, got %v", err)
	}
}

func TestApplyReceipt(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	d, r := parseDespatch(t), parseReceipt(t)
	var v uuid.UUID
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		var err error
		if v, err = tx.UpsertVendor(ctx, d.Supplier.VKN, d.Supplier.Title); err != nil {
			return err
		}
		if _, err := tx.InsertDespatch(ctx, v, d, "raw/d.xml"); err != nil {
			return err
		}
		no, err := tx.ApplyReceipt(ctx, v, r)
		if err != nil {
			return err
		}
		if no != d.Number {
			t.Errorf("despatchNo=%q", no)
		}
		var rej string
		if err := tx.QueryRow(ctx, `SELECT rejected_qty::text FROM despatch_lines WHERE line_id='1'`).Scan(&rej); err != nil {
			return err
		}
		if rej != "50.0000" {
			t.Errorf("rejected=%s", rej)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// her hata kendi transaction'ını bozar, o yüzden her biri ayrı InTenant'ta
	bad := *r
	bad.Lines = []ubl.ReceiptLine{{DespatchLineID: "99", RejectedQty: decimal.NewFromInt(1)}}
	err = s.InTenant(ctx, tid, func(tx *Tx) error { _, err := tx.ApplyReceipt(ctx, v, &bad); return err })
	if !errors.Is(err, ErrUnknownDespatch) {
		t.Errorf("unknown line: %v", err)
	}
	over := *r
	over.Lines = []ubl.ReceiptLine{{DespatchLineID: "1", RejectedQty: decimal.NewFromInt(5000)}}
	err = s.InTenant(ctx, tid, func(tx *Tx) error { _, err := tx.ApplyReceipt(ctx, v, &over); return err })
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("rejected > delivered: want 23514, got %v", err)
	}
}

func TestApplyReceiptUnknownDespatch(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	r := parseReceipt(t)
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		v, err := tx.UpsertVendor(ctx, "1234567890", "V")
		if err != nil {
			return err
		}
		_, err = tx.ApplyReceipt(ctx, v, r)
		return err
	})
	if !errors.Is(err, ErrUnknownDespatch) {
		t.Fatalf("want ErrUnknownDespatch, got %v", err)
	}
}

func TestUpsertVendorIdempotent(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		a, err := tx.UpsertVendor(ctx, "1234567890", "V")
		if err != nil {
			return err
		}
		b, err := tx.UpsertVendor(ctx, "1234567890", "V renamed")
		if err != nil {
			return err
		}
		if a != b {
			t.Errorf("ids differ: %s %s", a, b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInsertContractAndTenantVKN(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		if vkn, err := tx.TenantVKN(ctx); err != nil || len(vkn) != 10 {
			t.Errorf("vkn=%q err=%v", vkn, err)
		}
		v, err := tx.UpsertVendor(ctx, "1234567890", "V")
		if err != nil {
			return err
		}
		id, err := tx.InsertContract(ctx, ContractInput{
			VendorID: v, Number: "C-" + uuid.NewString(), PaymentTermDays: 30, Currency: "TRY",
			QtyTolerancePct: decimal.RequireFromString("2.50"),
			ValidFrom:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			ValidUntil:      time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			Items: []ContractItemInput{{ItemCode: "X", Name: "x", UOM: "C62", Currency: "TRY",
				UnitPrice: decimal.RequireFromString("1.25"), RequiresDelivery: true}},
		})
		if err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM contract_items WHERE contract_id=$1`, id).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("items=%d", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInTenantRollsBackOnError(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	boom := errors.New("boom")
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		if _, err := tx.UpsertVendor(ctx, "5555555555", "V"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	var n int
	if err := s.InTenant(ctx, tid, func(tx *Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM vendors`).Scan(&n)
	}); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestInTenantRollsBackOnPanic(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic not re-raised")
			}
		}()
		s.InTenant(ctx, tid, func(tx *Tx) error {
			if _, err := tx.UpsertVendor(ctx, "5555555555", "V"); err != nil {
				t.Error(err)
			}
			panic("x")
		})
	}()
	var n int
	if err := s.InTenant(ctx, tid, func(tx *Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM vendors`).Scan(&n)
	}); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestPriceHistoryWindowCurrencyUOM(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	target := parseInvoice(t)
	day := func(off int) time.Time { return target.IssueDate.AddDate(0, 0, off) }
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		v, err := tx.UpsertVendor(ctx, target.Supplier.VKN, target.Supplier.Title)
		if err != nil {
			return err
		}
		add := func(off int, cur, uom string) error {
			inv := parseInvoice(t)
			inv.Number, inv.IssueDate, inv.Currency, inv.Lines[0].UOM = uuid.NewString()[:12], day(off), cur, uom
			_, err := tx.InsertInvoice(ctx, v, inv, "")
			return err
		}
		uom := target.Lines[0].UOM
		for _, h := range []struct {
			off      int
			cur, uom string
		}{{-10, "TRY", uom}, {-180, "TRY", uom}, // dahil; -180 alt sınıra dahil
			{-10, "USD", uom}, {-181, "TRY", uom}, {0, "TRY", uom}, {-10, "TRY", "KGM"}} { // hepsi hariç
			if err := add(h.off, h.cur, h.uom); err != nil {
				return err
			}
		}
		id, err := tx.InsertInvoice(ctx, v, target, "")
		if err != nil {
			return err
		}
		pts, err := tx.PriceHistory(ctx, domain.Invoice{ID: id, VendorID: v, Currency: "TRY", IssueDate: target.IssueDate}, domain.InvoiceLine{ItemCode: target.Lines[0].ItemCode, UOM: uom})
		if err != nil {
			return err
		}
		if len(pts) != 2 || !pts[0].IssueDate.Equal(day(-180)) || !pts[1].IssueDate.Equal(day(-10)) {
			t.Errorf("history = %+v, want the -180d and -10d TRY points", pts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
