package repository

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

func draftContract(t *testing.T, ctx context.Context, tx *Tx) uuid.UUID {
	t.Helper()
	v, err := tx.UpsertVendor(ctx, "1234567890", "V")
	if err != nil {
		t.Fatal(err)
	}
	id, err := tx.InsertContract(ctx, ContractInput{
		VendorID: v, Number: "C-" + uuid.NewString(), PaymentTermDays: 30, Currency: "TRY",
		QtyTolerancePct: decimal.RequireFromString("2.50"),
		ValidFrom:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil:      time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func wantPgCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("want pg error %s, got %v", code, err)
	}
}

func TestDraftLifecycle(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	id := uuid.New()
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		if err := tx.InsertDraft(ctx, id, "p.pdf", "abc"); err != nil {
			return err
		}
		d, err := tx.GetDraft(ctx, id)
		if err != nil {
			return err
		}
		if d.Status != DraftExtracting || d.Draft != nil || d.PDFPath != "p.pdf" || d.PDFSHA256 != "abc" {
			t.Errorf("after insert: %+v", d)
		}
		if err := tx.SetDraftResult(ctx, id, json.RawMessage(`{"contract_no":"X"}`), json.RawMessage(`[]`), json.RawMessage(`{"pages":2}`)); err != nil {
			return err
		}
		d, err = tx.LockDraft(ctx, id)
		if err != nil {
			return err
		}
		var got map[string]string
		if err := json.Unmarshal(d.Draft, &got); err != nil || d.Status != DraftReady || got["contract_no"] != "X" {
			t.Errorf("after result: %+v err=%v", d, err)
		}
		cid := draftContract(t, ctx, tx)
		if err := tx.ApproveDraft(ctx, id, cid); err != nil {
			return err
		}
		d, err = tx.GetDraft(ctx, id)
		if err != nil {
			return err
		}
		if d.Status != DraftApproved || d.ContractID == nil || *d.ContractID != cid {
			t.Errorf("after approve: %+v", d)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDraftFailedAndRetry(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	id := uuid.New()
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		if err := tx.InsertDraft(ctx, id, "p.pdf", "abc"); err != nil {
			return err
		}
		if err := tx.SetDraftFailed(ctx, id, "ocr_failed"); err != nil {
			return err
		}
		if d, _ := tx.GetDraft(ctx, id); d.Status != DraftFailed || d.ErrorCode != "ocr_failed" {
			t.Errorf("failed: %+v", d)
		}
		if err := tx.SetDraftExtracting(ctx, id); err != nil {
			return err
		}
		if d, _ := tx.GetDraft(ctx, id); d.Status != DraftExtracting {
			t.Errorf("retry: %+v", d)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDraftRLS(t *testing.T) {
	s, a := setup(t)
	ctx := context.Background()
	b, err := s.CreateTenant(ctx, "B", uuid.NewString()[:8]+"00", decimal.RequireFromString("0.4500"))
	if err != nil {
		t.Fatal(err)
	}
	cleanupTenant(t, b)
	id := uuid.New()
	if err := s.InTenant(ctx, a, func(tx *Tx) error { return tx.InsertDraft(ctx, id, "p", "h") }); err != nil {
		t.Fatal(err)
	}
	err = s.InTenant(ctx, b, func(tx *Tx) error { _, err := tx.GetDraft(ctx, id); return err })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDraftApproveRequiresContract(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	id := uuid.New()
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		if err := tx.InsertDraft(ctx, id, "p", "h"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE contract_drafts SET status='APPROVED' WHERE id=$1`, id)
		return err
	})
	wantPgCode(t, err, "23514")
}

func TestDraftCrossTenantContractFK(t *testing.T) {
	s, a := setup(t)
	ctx := context.Background()
	b, err := s.CreateTenant(ctx, "B", uuid.NewString()[:8]+"00", decimal.RequireFromString("0.4500"))
	if err != nil {
		t.Fatal(err)
	}
	cleanupTenant(t, b)
	var bContract uuid.UUID
	if err := s.InTenant(ctx, b, func(tx *Tx) error { bContract = draftContract(t, ctx, tx); return nil }); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	err = s.InTenant(ctx, a, func(tx *Tx) error {
		if err := tx.InsertDraft(ctx, id, "p", "h"); err != nil {
			return err
		}
		return tx.ApproveDraft(ctx, id, bContract)
	})
	wantPgCode(t, err, "23503")
}

// cleanupTenant removes an extra tenant created by a test (owner role, GUC set because FORCE RLS applies).
func cleanupTenant(t *testing.T, tid uuid.UUID) {
	t.Cleanup(func() {
		ctx := context.Background()
		owner, err := pgx.Connect(ctx, os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"))
		if err != nil {
			return
		}
		defer owner.Close(ctx)
		tx, err := owner.Begin(ctx)
		if err != nil {
			return
		}
		defer tx.Rollback(ctx)
		tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", tid.String())
		for _, tbl := range []string{"contract_drafts", "contracts", "vendors"} {
			tx.Exec(ctx, "DELETE FROM "+tbl+" WHERE tenant_id=$1", tid)
		}
		tx.Exec(ctx, "DELETE FROM tenants WHERE id=$1", tid)
		tx.Commit(ctx)
	})
}
