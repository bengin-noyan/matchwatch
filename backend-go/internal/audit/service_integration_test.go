package audit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/matcher"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/migrate"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/ubl"
)

type env struct {
	store *repository.Store
	tid   uuid.UUID
	svc   *Service
	vendr uuid.UUID
}

func open(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// setup creates a fresh tenant (unique VKN per run) with the basic contract, despatch and receipt loaded.
func setup(t *testing.T) *env {
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
	s := repository.NewStore(pool)
	tid, err := s.CreateTenant(ctx, "audit-"+uuid.NewString(), "9876543210", decimal.RequireFromString("0.4500"))
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
		for _, tbl := range []string{"invoices", "despatch_advices", "contracts", "vendors"} {
			tx.Exec(ctx, "DELETE FROM "+tbl+" WHERE tenant_id=$1", tid)
		}
		tx.Exec(ctx, "DELETE FROM tenants WHERE id=$1", tid)
		tx.Commit(ctx)
	})
	e := &env{store: s, tid: tid, svc: New(s, nil)}

	doc, err := repository.ParseContractJSON(open(t, "../../testdata/contract_basic.json"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := ubl.ParseDespatch(open(t, "../../testdata/despatch_basic.xml"))
	if err != nil {
		t.Fatal(err)
	}
	d.ETTN = uuid.NewString()
	r, err := ubl.ParseReceipt(open(t, "../../testdata/receipt_basic.xml"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.InTenant(ctx, tid, func(tx *repository.Tx) error {
		if e.vendr, err = tx.UpsertVendor(ctx, doc.VendorVKN, doc.VendorName); err != nil {
			return err
		}
		doc.VendorID = e.vendr
		if _, err = tx.InsertContract(ctx, doc.ContractInput); err != nil {
			return err
		}
		if _, err = tx.InsertDespatch(ctx, e.vendr, d, ""); err != nil {
			return err
		}
		_, err = tx.ApplyReceipt(ctx, e.vendr, r)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// ingest stores invoice_basic.xml with a unique ETTN/number; qty900 shrinks line 1 to 900 units at 1.30.
func (e *env) ingest(t *testing.T, qty900 bool) uuid.UUID {
	t.Helper()
	inv, err := ubl.ParseInvoice(open(t, "../../testdata/invoice_basic.xml"))
	if err != nil {
		t.Fatal(err)
	}
	inv.ETTN = uuid.NewString()
	inv.Number = "T" + uuid.NewString()[:12]
	if qty900 {
		inv.Lines[0].Qty = decimal.NewFromInt(900)
		inv.Lines[0].LineExtension = decimal.RequireFromString("1170.00")
		inv.Lines[0].Allowance = decimal.Zero
		inv.Payable = decimal.RequireFromString("1234.56") // differs from other invoices: no amount duplicate
	}
	var id uuid.UUID
	err = e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		var err error
		id, err = tx.InsertInvoice(context.Background(), e.vendr, inv, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type row struct {
	rule     domain.RuleCode
	lineID   string // invoice line_id, "" for header findings
	impact   int64
	status   string
	evidence map[string]any
}

func (e *env) findings(t *testing.T, inv uuid.UUID) []row {
	t.Helper()
	var out []row
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT f.rule_code, COALESCE(l.line_id,''), f.financial_impact_minor,
			f.resolution_status, f.evidence_json FROM audit_findings f
			LEFT JOIN invoice_lines l ON l.tenant_id=f.tenant_id AND l.id=f.invoice_line_id
			WHERE f.invoice_id=$1 ORDER BY f.rule_code, l.line_id`, inv)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.rule, &r.lineID, &r.impact, &r.status, &r.evidence); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), q, args...).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		_, err := tx.Exec(context.Background(), q, args...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (e *env) audit(t *testing.T, id uuid.UUID) domain.AuditStatus {
	t.Helper()
	st, err := e.svc.AuditInvoice(context.Background(), e.tid, id)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func sig(rs []row) []string {
	var s []string
	for _, r := range rs {
		s = append(s, fmt.Sprintf("%s/%s/%d", r.rule, r.lineID, r.impact))
	}
	return s
}

func TestAuditBasicInvoice(t *testing.T) {
	e := setup(t)
	inv := e.ingest(t, false)
	if st := e.audit(t, inv); st != domain.AuditRed {
		t.Fatalf("status %s", st)
	}
	got := fmt.Sprint(sig(e.findings(t, inv)))
	want := "[MATURITY_VIOLATION//9912 PRICE_MISMATCH/1/5000 QTY_EXCEEDS_DELIVERY/1/6500]"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	for _, r := range e.findings(t, inv) {
		if r.rule == domain.RuleMaturityViolation {
			if r.evidence["source_page"] != float64(2) || r.evidence["contract_no"] != "S-2026-001" ||
				r.evidence["source_clause_text"] != "Madde 3.2: Ödeme vadesi fatura tarihinden itibaren 60 gündür." {
				t.Errorf("maturity evidence %+v", r.evidence)
			}
		}
	}
	var st string
	if err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT audit_status FROM invoices WHERE id=$1`, inv).Scan(&st)
	}); err != nil || st != "RED" {
		t.Fatalf("persisted status %q err %v", st, err)
	}
}

func TestAuditIdempotent(t *testing.T) {
	e := setup(t)
	inv := e.ingest(t, false)
	const allocQ = `SELECT count(*) FROM despatch_allocations a JOIN invoice_lines l ON l.id=a.invoice_line_id WHERE l.invoice_id=$1`
	e.audit(t, inv)
	f1 := e.count(t, `SELECT count(*) FROM audit_findings WHERE invoice_id=$1`, inv)
	a1 := e.count(t, allocQ, inv)
	e.audit(t, inv)
	f2 := e.count(t, `SELECT count(*) FROM audit_findings WHERE invoice_id=$1`, inv)
	a2 := e.count(t, allocQ, inv)
	if f1 != 3 || f1 != f2 || a1 != 1 || a1 != a2 {
		t.Fatalf("findings %d/%d allocations %d/%d", f1, f2, a1, a2)
	}
}

func TestAuditKeepsDisputed(t *testing.T) {
	e := setup(t)
	inv := e.ingest(t, false)
	e.audit(t, inv)
	e.exec(t, `UPDATE audit_findings SET resolution_status='DISPUTED' WHERE invoice_id=$1 AND rule_code='PRICE_MISMATCH'`, inv)
	if st := e.audit(t, inv); st != domain.AuditRed {
		t.Fatalf("status %s", st)
	}
	n := 0
	for _, r := range e.findings(t, inv) {
		if r.rule == domain.RulePriceMismatch {
			n++
			if r.status != "DISPUTED" {
				t.Fatalf("status %s", r.status)
			}
		}
	}
	if n != 1 {
		t.Fatalf("PRICE_MISMATCH rows: %d", n)
	}
}

func TestAuditApprovedExceptionNotCounted(t *testing.T) {
	e := setup(t)
	inv := e.ingest(t, false)
	e.audit(t, inv)
	e.exec(t, `UPDATE audit_findings SET resolution_status='APPROVED_EXCEPTION' WHERE invoice_id=$1 AND severity IN ('CRITICAL','WARNING')`, inv)
	if st := e.audit(t, inv); st != domain.AuditGreen {
		t.Fatalf("status %s", st)
	}
}

func TestAuditSecondInvoiceSameDespatch(t *testing.T) {
	e := setup(t)
	first := e.ingest(t, true)
	e.audit(t, first)
	for _, r := range e.findings(t, first) {
		if r.rule == domain.RuleQtyExceedsDelivery {
			t.Fatalf("unexpected qty finding on first invoice: %v", r)
		}
	}
	second := e.ingest(t, true)
	e.audit(t, second)
	found := false
	for _, r := range e.findings(t, second) {
		if r.rule == domain.RuleQtyExceedsDelivery {
			found = true
			if r.evidence["available_total"] != "50" {
				t.Fatalf("available_total %v", r.evidence["available_total"])
			}
		}
	}
	if !found {
		t.Fatal("no QTY_EXCEEDS_DELIVERY on second invoice")
	}
}

// TestAuditConcurrentSameDespatch: tx A holds the despatch-line lock with its allocations inserted but not yet
// committed while audit B starts and blocks on the lock. After A commits, B must see A's allocations.
func TestAuditConcurrentSameDespatch(t *testing.T) {
	e := setup(t)
	a, b := e.ingest(t, true), e.ingest(t, true)
	ctx := context.Background()
	started := make(chan struct{})
	done := make(chan error, 1)
	err := e.store.InTenant(ctx, e.tid, func(tx *repository.Tx) error {
		if _, err := tx.ResetAudit(ctx, a); err != nil {
			return err
		}
		inv, err := tx.LoadInvoice(ctx, a)
		if err != nil {
			return err
		}
		cands, err := tx.DespatchCandidates(ctx, inv) // takes the lock
		if err != nil {
			return err
		}
		var allocs []domain.Allocation
		for _, l := range inv.Lines {
			allocs = append(allocs, matcher.Allocate(l, cands).Allocations...)
		}
		if err := tx.SaveAudit(ctx, a, nil, allocs, nil, domain.AuditGreen); err != nil {
			return err
		}
		go func() {
			close(started)
			_, err := e.svc.AuditInvoice(ctx, e.tid, b)
			done <- err
		}()
		<-started
		time.Sleep(500 * time.Millisecond) // let B reach the lock wait before A commits
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	total := e.count(t, `SELECT COALESCE(SUM(a.qty),0)::int FROM despatch_allocations a JOIN invoice_lines l ON l.id=a.invoice_line_id
		WHERE l.invoice_id IN ($1,$2)`, a, b)
	if total > 950 {
		t.Fatalf("allocated %d > 950 delivered", total)
	}
	qty := 0
	for _, id := range []uuid.UUID{b} {
		for _, r := range e.findings(t, id) {
			if r.rule == domain.RuleQtyExceedsDelivery {
				qty++
			}
		}
	}
	if qty != 1 {
		t.Fatalf("expected QTY_EXCEEDS_DELIVERY on B, got %d", qty)
	}
}

// An invoice that references no despatch draws on the vendor's last 30 days of despatches (regression: the lock
// statement left the invoice-id parameter untyped and the audit failed with SQLSTATE 42P18).
func TestAuditInvoiceWithoutDespatchRefs(t *testing.T) {
	e := setup(t)
	inv, err := ubl.ParseInvoice(open(t, "../../testdata/invoice_basic.xml"))
	if err != nil {
		t.Fatal(err)
	}
	inv.ETTN, inv.Number, inv.DespatchRefs = uuid.NewString(), "T"+uuid.NewString()[:12], nil
	var id uuid.UUID
	err = e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		id, err = tx.InsertInvoice(context.Background(), e.vendr, inv, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e.audit(t, id)
	if got := fmt.Sprint(sig(e.findings(t, id))); got != "[MATURITY_VIOLATION//9912 PRICE_MISMATCH/1/5000 QTY_EXCEEDS_DELIVERY/1/6500]" {
		t.Fatalf("got %s", got)
	}
}
