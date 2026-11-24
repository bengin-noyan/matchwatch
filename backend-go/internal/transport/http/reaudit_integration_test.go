package transporthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
	"github.com/bengin-noyan/matchwatch/backend-go/migrations"
)

// leavingEmb, denetimin ortasında isteği iptal eder: değişiklik commit edilmiş, denetim yarım kalmış olur.
type leavingEmb struct{ leave func() }

func (l *leavingEmb) Embed(context.Context, []string) (string, []json.RawMessage, error) {
	if l.leave != nil {
		l.leave()
	}
	return "", nil, errors.New("öneri yok")
}

func (e *env) sql(q string, args ...any) {
	e.t.Helper()
	if err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		_, err := tx.Exec(context.Background(), q, args...)
		return err
	}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) waiting(inv string) (b bool) {
	e.t.Helper()
	if err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT reaudit_since IS NOT NULL FROM invoices WHERE id=$1`, inv).Scan(&b)
	}); err != nil {
		e.t.Fatal(err)
	}
	return
}

// Yeniden denetimi yarım kalan fatura işaretli kalır ve tarama onu toparlar; başarılı denetim işaret bırakmaz.
func TestFailedReauditIsRetried(t *testing.T) {
	e := setup(t, "9876543210")
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	if _, err := owner.Exec(ctx, `INSERT INTO users(tenant_id, email, password_hash) VALUES ($1,$2,'x')`,
		e.tid, uuid.NewString()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	emb := &leavingEmb{}
	svc := audit.New(e.store, emb, nil)
	e.srv = NewRouter(e.store, svc, e.storage, secret, nil, 0, false, e.mails, "https://mw.test")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/invoices", fixture(t, "invoice_basic.xml", invETTN, uuid.NewString(),
		"ABC2026000000001", "T"+uuid.NewString()[:12]))
	e.mustStatus(201, code, b)
	inv := str(b["id"])
	// sözleşmede olmayan bir satır: her denetim Embed'i çağırır
	e.sql(`INSERT INTO invoice_lines(tenant_id, invoice_id, line_id, item_name, uom, qty, unit_price, line_extension_minor)
		VALUES ($1,$2,'99','bilinmeyen kalem','C62',1,1,100)`, e.tid, inv)
	e.sql(`UPDATE invoice_lines SET item_code='', barcode='' WHERE invoice_id=$1 AND line_id='1'`, inv)
	line := e.id(`SELECT id FROM invoice_lines WHERE invoice_id=$1 AND line_id='1'`, inv)
	item := e.id(`SELECT ci.id FROM contract_items ci JOIN contracts c ON c.id=ci.contract_id
		WHERE c.contract_no='S-2026-001' AND ci.item_code='CV-M8-20'`)
	receipt := fixture(t, "receipt_basic.xml")

	var alias string
	for _, step := range []struct {
		name, method, path string
		body               []byte
	}{
		{"despatch", "POST", "/api/v1/despatches", despatchXML(t)},
		{"receipt", "POST", "/api/v1/receipts", receipt},
		{"match", "POST", "/api/v1/invoice-lines/" + line.String() + "/match", []byte(`{"contract_item_id":"` + item.String() + `"}`)},
		{"alias delete", "DELETE", "/api/v1/aliases/", nil},
	} {
		if step.name == "alias delete" {
			alias = e.id(`SELECT id FROM item_aliases`).String()
			step.path += alias
		}
		rctx, cancel := context.WithCancel(ctx)
		emb.leave = cancel
		r := httptest.NewRequest(step.method, step.path, bytes.NewReader(step.body)).WithContext(rctx)
		r.Header.Set("Authorization", "Bearer "+e.token)
		rec := httptest.NewRecorder()
		e.srv.ServeHTTP(rec, r)
		cancel()
		emb.leave = nil
		if rec.Code < 500 || !e.waiting(inv) {
			t.Fatalf("%s: code %d %s, waiting %v", step.name, rec.Code, rec.Body, e.waiting(inv))
		}
		if err := svc.RetryPending(ctx); err != nil || !e.waiting(inv) { // daha yeni: isteğin kendisi sürüyor olabilir
			t.Fatalf("%s: err %v, retried too early", step.name, err)
		}
		e.sql(`UPDATE invoices SET reaudit_since = now() - interval '1 hour' WHERE id=$1`, inv)
		if err := svc.RetryPending(ctx); err != nil || e.waiting(inv) {
			t.Fatalf("%s: err %v, waiting %v after the retry", step.name, err, e.waiting(inv))
		}
		matched := e.id(`SELECT COALESCE(matched_contract_item_id, $2) FROM invoice_lines WHERE id=$1`, line, uuid.Nil) == item
		if matched != (step.name == "match") {
			t.Fatalf("%s: line matched=%v after the retry", step.name, matched)
		}
	}
	code, b = e.req("POST", "/api/v1/receipts", receipt)
	e.mustStatus(200, code, b)
	if e.waiting(inv) {
		t.Fatal("a successful re-audit left the mark")
	}
}

// Denetim önce faturayı, sonra irsaliye satırlarını kilitler. İrsaliye yanıtı aynı sırayı izlemezse ikisi birbirini
// bekler.
func TestReceiptLocksInvoicesFirst(t *testing.T) {
	e := setup(t, "9876543210")
	ctx := context.Background()
	e.loadContract()
	code, b := e.req("POST", "/api/v1/despatches", despatchXML(t))
	e.mustStatus(201, code, b)
	code, b = e.req("POST", "/api/v1/invoices", fixture(t, "invoice_basic.xml", invETTN, uuid.NewString(),
		"ABC2026000000001", "T"+uuid.NewString()[:12]))
	e.mustStatus(201, code, b)
	receipt := fixture(t, "receipt_basic.xml")
	done := make(chan int, 1)
	err := e.store.InTenant(ctx, e.tid, func(tx *repository.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM invoices WHERE id=$1 FOR UPDATE`, str(b["id"])); err != nil {
			return err
		}
		go func() {
			code, _ := e.req("POST", "/api/v1/receipts", receipt)
			done <- code
		}()
		time.Sleep(300 * time.Millisecond) // yanıtın fatura kilidinde beklemeye başlaması için
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `SELECT id FROM despatch_lines FOR UPDATE`)
		return err
	})
	if err != nil {
		t.Error(err)
	}
	if code := <-done; code != 200 {
		t.Errorf("receipt: %d", code)
	}
}

// Sözleşme kaydedilince, o tedarikçinin geçerlilik aralığındaki faturaları yeniden denetlenmek üzere işaretlenir.
func TestContractMarksInvoices(t *testing.T) {
	e := setup(t, "9876543210")
	post := func(repl ...string) string {
		t.Helper()
		code, b := e.req("POST", "/api/v1/invoices", fixture(t, "invoice_basic.xml", append([]string{invETTN, uuid.NewString(),
			"ABC2026000000001", "T" + uuid.NewString()[:12]}, repl...)...))
		e.mustStatus(201, code, b)
		return str(b["id"])
	}
	inv, old, otherVendor := post(), post(), post()
	e.sql(`UPDATE invoices SET issue_date='2001-01-01' WHERE id=$1`, old)
	e.sql(`WITH v AS (INSERT INTO vendors(tenant_id, vkn, name) VALUES ($1,'1111111111','Başka') RETURNING id)
		UPDATE invoices SET vendor_id=(SELECT id FROM v) WHERE id=$2`, e.tid, otherVendor)
	e.loadContract()
	if got := []bool{e.waiting(inv), e.waiting(old), e.waiting(otherVendor)}; !slices.Equal(got, []bool{true, false, false}) {
		t.Fatalf("waiting %v", got)
	}
	for id, want := range map[string]bool{inv: true, old: false} {
		code, b := e.req("GET", "/api/v1/invoices/"+id, nil)
		e.mustStatus(200, code, b)
		if b["reaudit_pending"] != want {
			t.Errorf("reaudit_pending %v want %v", b["reaudit_pending"], want)
		}
	}
	code, b := e.req("POST", "/api/v1/invoices/"+inv+"/audit", nil)
	e.mustStatus(200, code, b)
	if e.waiting(inv) {
		t.Error("the audit left the mark")
	}
}

// 017, işaretlerden önce yarım kalmış olabilecek yeniden denetimler için eski faturaları işaretler.
func TestMigrationMarksOldInvoices(t *testing.T) {
	e := setup(t, "9876543210")
	ctx := context.Background()
	code, b := e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(201, code, b)
	owner, err := pgx.Connect(ctx, os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	sql, err := migrations.FS.ReadFile("017_reaudit_backfill.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) // diğer testlerin faturaları işaretli kalmasın
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout='10s'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users(tenant_id, email, password_hash) VALUES ($1,$2,'x')`,
		e.tid, uuid.NewString()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	var marked bool
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", e.tid.String()); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT reaudit_since IS NOT NULL FROM invoices WHERE id=$1`, str(b["id"])).Scan(&marked); err != nil || !marked {
		t.Fatalf("marked %v err %v", marked, err)
	}
}
