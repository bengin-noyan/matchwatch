package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/ubl"
)

// fakeEmbedder, "civata" adlarını bir eksene, "bakim" adlarını başka bir eksene yakın koyar; geri kalan her şey uzak
// düşer.
type fakeEmbedder struct {
	calls [][]string
	err   error
}

func axisVec(lead ...string) json.RawMessage {
	parts := make([]string, 1024)
	for i := range parts {
		parts[i] = "0"
		if i < len(lead) {
			parts[i] = lead[i]
		}
	}
	return json.RawMessage("[" + strings.Join(parts, ",") + "]")
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) (string, []json.RawMessage, error) {
	f.calls = append(f.calls, texts)
	if f.err != nil {
		return "", nil, f.err
	}
	out := make([]json.RawMessage, len(texts))
	for i, t := range texts {
		switch {
		case strings.Contains(t, "civata"):
			out[i] = axisVec("1", "0.1")
		case strings.Contains(t, "bakim"):
			out[i] = axisVec("0", "1")
		default:
			out[i] = axisVec("0", "0", "1")
		}
	}
	return "fake-m", out, nil
}

// ingestCodeless, invoice_basic.xml'i 1. satırın kalem kodu ve barkodu silinmiş haliyle kaydeder.
func (e *env) ingestCodeless(t *testing.T) (invoiceID, line1 uuid.UUID) {
	t.Helper()
	inv, err := ubl.ParseInvoice(open(t, "../../testdata/invoice_basic.xml"))
	if err != nil {
		t.Fatal(err)
	}
	inv.ETTN, inv.Number = uuid.NewString(), "T"+uuid.NewString()[:12]
	inv.Lines[0].ItemCode, inv.Lines[0].Barcode = "", ""
	err = e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		var err error
		if invoiceID, err = tx.InsertInvoice(context.Background(), e.vendr, inv, ""); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `SELECT id FROM invoice_lines WHERE invoice_id=$1 AND line_id='1'`, invoiceID).Scan(&line1)
	})
	if err != nil {
		t.Fatal(err)
	}
	return invoiceID, line1
}

func ruleRow(rows []row, rule domain.RuleCode, line string) *row {
	for i := range rows {
		if rows[i].rule == rule && rows[i].lineID == line {
			return &rows[i]
		}
	}
	return nil
}

func TestSemanticSuggestionThenAlias(t *testing.T) {
	e := setup(t)
	emb := &fakeEmbedder{}
	e.svc = New(e.store, emb, nil)
	inv, line1 := e.ingestCodeless(t)
	e.audit(t, inv)

	u := ruleRow(e.findings(t, inv), domain.RuleUncontractedItem, "1")
	if u == nil {
		t.Fatalf("no UNCONTRACTED_ITEM: %v", sig(e.findings(t, inv)))
	}
	sug, _ := u.evidence["suggestions"].([]any)
	if len(sug) != 1 {
		t.Fatalf("suggestions %+v", u.evidence)
	}
	top := sug[0].(map[string]any)
	if top["item_name"] != "M8x20 paslanmaz civata" || top["similarity"] != "1.0000" {
		t.Fatalf("top suggestion %+v", top)
	}
	// önce yalnızca kodsuz satır, sonra iki sözleşme kalemi birer kez embed edilir
	if len(emb.calls) != 2 || fmt.Sprint(emb.calls[0]) != "[m8 20mm inox civata]" || len(emb.calls[1]) != 2 {
		t.Fatalf("calls %v", emb.calls)
	}
	e.audit(t, inv)
	if len(emb.calls) != 3 { // kalem vektörleri kaydedildi: yeniden denetimde yalnızca satır embed edilir
		t.Fatalf("re-audit calls %v", emb.calls)
	}

	itemID, _ := uuid.Parse(top["contract_item_id"].(string))
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		_, err := tx.UpsertAlias(context.Background(), line1, itemID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if st := e.audit(t, inv); st != domain.AuditRed {
		t.Fatalf("status %s", st)
	}
	rows := e.findings(t, inv)
	// kodlu faturayla aynı sonuç: onaylı eşleşme çalışıyor ve irsaliyeler sözleşme kaleminin koduyla bulunuyor
	if got := fmt.Sprint(sig(rows)); got != "[MATURITY_VIOLATION//9912 PRICE_MISMATCH/1/5000 QTY_EXCEEDS_DELIVERY/1/6500]" {
		t.Fatalf("after alias: %s", got)
	}
	if p := ruleRow(rows, domain.RulePriceMismatch, "1"); p.evidence["match_method"] != "ALIAS" {
		t.Fatalf("price evidence %+v", p.evidence)
	}
	if len(emb.calls) != 3 {
		t.Fatalf("aliased line was embedded: %v", emb.calls)
	}
}

func TestSemanticAIErrorKeepsFaz1Result(t *testing.T) {
	e := setup(t)
	e.svc = New(e.store, &fakeEmbedder{err: errors.New("down")}, nil)
	inv, _ := e.ingestCodeless(t)
	withAI := e.audit(t, inv)
	got := sig(e.findings(t, inv))

	e.svc = New(e.store, nil, nil)
	if st := e.audit(t, inv); st != withAI || fmt.Sprint(sig(e.findings(t, inv))) != fmt.Sprint(got) {
		t.Fatalf("AI error changed the result: %s %v vs %s %v", withAI, got, st, sig(e.findings(t, inv)))
	}
	if u := ruleRow(e.findings(t, inv), domain.RuleUncontractedItem, "1"); u == nil || u.evidence["suggestions"] != nil {
		t.Fatalf("uncontracted %+v", u)
	}
}
