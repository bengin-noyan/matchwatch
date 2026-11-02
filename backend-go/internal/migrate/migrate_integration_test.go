package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bengin-noyan/matchwatch/backend-go/migrations"
)

func connect(t *testing.T, env string) *pgx.Conn {
	t.Helper()
	url := os.Getenv(env)
	if url == "" {
		t.Skipf("%s not set", env)
	}
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

// inTenant runs fn in a transaction with the tenant GUC set (tenant "" = set to empty).
func inTenant(t *testing.T, conn *pgx.Conn, tenant string, fn func(tx pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", tenant); err != nil {
		t.Fatal(err)
	}
	fn(tx)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, tx pgx.Tx, q string) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// setup migrates, creates tenants A and B (owner role) and a vendor for A.
func setup(t *testing.T) (owner, app *pgx.Conn, a, b uuid.UUID) {
	t.Helper()
	owner = connect(t, "MATCHWATCH_TEST_MIGRATE_URL")
	app = connect(t, "MATCHWATCH_TEST_DATABASE_URL")
	ctx := context.Background()
	if err := Apply(ctx, owner); err != nil {
		t.Fatal(err)
	}
	a, b = uuid.New(), uuid.New()
	suffix := fmt.Sprint(time.Now().UnixNano())[9:]
	for i, id := range []uuid.UUID{a, b} {
		inTenant(t, owner, id.String(), func(tx pgx.Tx) {
			_, err := tx.Exec(ctx, `INSERT INTO tenants(id, name, vkn) VALUES ($1, 'T', $2)`, id, fmt.Sprintf("%d%s", i, suffix))
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	inTenant(t, owner, a.String(), func(tx pgx.Tx) {
		if _, err := tx.Exec(ctx, `INSERT INTO vendors(tenant_id, vkn, name) VALUES ($1, '1234567890', 'V')`, a); err != nil {
			t.Fatal(err)
		}
	})
	t.Cleanup(func() {
		for _, id := range []uuid.UUID{a, b} {
			tx, err := owner.Begin(ctx)
			if err != nil {
				return
			}
			tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", id.String())
			tx.Exec(ctx, "DELETE FROM invoice_lines WHERE tenant_id=$1", id)
			tx.Exec(ctx, "DELETE FROM invoices WHERE tenant_id=$1", id)
			tx.Exec(ctx, "DELETE FROM vendors WHERE tenant_id=$1", id)
			tx.Exec(ctx, "DELETE FROM tenants WHERE id=$1", id)
			tx.Commit(ctx)
		}
	})
	return
}

func TestMigrateIdempotent(t *testing.T) {
	owner := connect(t, "MATCHWATCH_TEST_MIGRATE_URL")
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := Apply(ctx, owner); err != nil {
			t.Fatal(err)
		}
	}
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := owner.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(files) {
		t.Fatalf("schema_migrations rows = %d, want %d", n, len(files))
	}
}

func TestRLSIsolation(t *testing.T) {
	_, app, a, b := setup(t)
	inTenant(t, app, b.String(), func(tx pgx.Tx) {
		if n := count(t, tx, "SELECT count(*) FROM vendors"); n != 0 {
			t.Fatalf("tenant B sees %d vendors", n)
		}
	})
	inTenant(t, app, a.String(), func(tx pgx.Tx) {
		if n := count(t, tx, "SELECT count(*) FROM vendors"); n != 1 {
			t.Fatalf("tenant A sees %d vendors, want 1", n)
		}
	})
}

func TestRLSNoTenantSeesNothing(t *testing.T) {
	_, app, _, _ := setup(t)
	ctx := context.Background()
	var n int
	if err := app.QueryRow(ctx, "SELECT count(*) FROM vendors").Scan(&n); err != nil || n != 0 {
		t.Fatalf("unset GUC: n=%d err=%v", n, err)
	}
	// After a transaction-local set_config the GUC lingers as '' on the session.
	inTenant(t, app, "", func(tx pgx.Tx) {
		if n := count(t, tx, "SELECT count(*) FROM vendors"); n != 0 {
			t.Fatalf("empty GUC: n=%d", n)
		}
	})
}

func TestRLSWithCheck(t *testing.T) {
	_, app, a, b := setup(t)
	ctx := context.Background()
	tx, err := app.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", a.String()); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO vendors(tenant_id, vkn, name) VALUES ($1, '9999999999', 'X')`, b)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("want SQLSTATE 42501, got %v", err)
	}
}

func TestRLSForcedForOwner(t *testing.T) {
	owner, _, _, _ := setup(t)
	var n int
	if err := owner.QueryRow(context.Background(), "SELECT count(*) FROM vendors").Scan(&n); err != nil || n != 0 {
		t.Fatalf("owner without GUC: n=%d err=%v", n, err)
	}
}

func TestWithholdingCodesSeeded(t *testing.T) {
	_, app, _, _ := setup(t)
	ctx := context.Background()
	var p string
	if err := app.QueryRow(ctx, "SELECT percent::text FROM withholding_codes WHERE code='601'").Scan(&p); err != nil || p != "40.00" {
		t.Fatalf("601 percent=%q err=%v", p, err)
	}
	var n int
	if err := app.QueryRow(ctx, "SELECT count(*) FROM withholding_codes").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n <= 10 {
		t.Fatalf("only %d codes", n)
	}
	tx, err := app.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "DELETE FROM withholding_codes"); err == nil {
		t.Fatal("app can delete withholding_codes")
	}
}

func TestCompositeFKBlocksCrossTenantLinks(t *testing.T) {
	owner, app, a, b := setup(t)
	ctx := context.Background()
	var bVendor, bInvoice, aVendor uuid.UUID
	inTenant(t, owner, b.String(), func(tx pgx.Tx) {
		if err := tx.QueryRow(ctx, `INSERT INTO vendors(tenant_id, vkn, name) VALUES ($1, '2222222222', 'BV') RETURNING id`, b).Scan(&bVendor); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO invoices(tenant_id, vendor_id, ettn, invoice_no, issue_date, currency, payable_minor)
			VALUES ($1, $2, gen_random_uuid(), 'B1', '2026-01-01', 'TRY', 100) RETURNING id`, b, bVendor).Scan(&bInvoice); err != nil {
			t.Fatal(err)
		}
	})
	inTenant(t, owner, a.String(), func(tx pgx.Tx) {
		if err := tx.QueryRow(ctx, `SELECT id FROM vendors WHERE tenant_id=$1`, a).Scan(&aVendor); err != nil {
			t.Fatal(err)
		}
	})
	fkViolation := func(q string, args ...any) {
		t.Helper()
		tx, err := app.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", a.String()); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, q, args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Fatalf("want FK violation 23503, got %v", err)
		}
	}
	fkViolation(`INSERT INTO invoices(tenant_id, vendor_id, ettn, invoice_no, issue_date, currency, payable_minor)
		VALUES ($1, $2, gen_random_uuid(), 'A1', '2026-01-01', 'TRY', 100)`, a, bVendor)
	fkViolation(`INSERT INTO invoice_lines(tenant_id, invoice_id, line_id, qty, unit_price, line_extension_minor)
		VALUES ($1, $2, '1', 1, 1, 100)`, a, bInvoice)
	// Control: the same insert with A's own vendor is accepted.
	inTenant(t, app, a.String(), func(tx pgx.Tx) {
		if _, err := tx.Exec(ctx, `INSERT INTO invoices(tenant_id, vendor_id, ettn, invoice_no, issue_date, currency, payable_minor)
			VALUES ($1, $2, gen_random_uuid(), 'A1', '2026-01-01', 'TRY', 100)`, a, aVendor); err != nil {
			t.Fatal(err)
		}
	})
}
