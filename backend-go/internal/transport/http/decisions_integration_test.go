package transporthttp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

func TestDecisionHistory(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	email := "karar-" + uuid.NewString()[:8] + "@firma.com.tr"
	uid := e.addUser(email, "dogru-sifre-123")
	userTok, err := auth.Sign(secret, e.tid, uid.String(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	code, b := e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(201, code, b)
	inv := "/api/v1/invoices/" + str(b["id"])
	code, b = e.req("GET", inv, nil)
	e.mustStatus(200, code, b)
	first := obj(list(b["findings"])[0])
	fid, rule, line := str(first["id"]), str(first["rule_code"]), first["line_id"]

	code, b = e.do("PATCH", "/api/v1/findings/"+fid, userTok, []byte(`{"resolution_status":"DISPUTED","note":"tedarikçi iade edecek"}`))
	e.mustStatus(200, code, b)
	code, b = e.req("PATCH", "/api/v1/findings/"+fid, []byte(`{"resolution_status":"OPEN"}`)) // dev token: no user
	e.mustStatus(200, code, b)
	// the re-audit deletes the reopened finding and recreates it under a new id; its trail must follow
	code, b = e.req("POST", inv+"/audit", nil)
	e.mustStatus(200, code, b)

	code, b = e.req("GET", inv, nil)
	e.mustStatus(200, code, b)
	var got map[string]any
	for _, f := range list(b["findings"]) {
		f := obj(f)
		if str(f["rule_code"]) == rule && f["line_id"] == line {
			got = f
		} else if n := len(list(f["decisions"])); n != 0 {
			t.Errorf("%s %v: %d decisions, want 0", str(f["rule_code"]), f["line_id"], n)
		}
	}
	if got == nil {
		t.Fatalf("finding %s %v gone after re-audit", rule, line)
	}
	if str(got["id"]) == fid {
		t.Fatalf("re-audit kept the OPEN finding's id; the test no longer covers recreation")
	}
	ds := list(got["decisions"])
	if len(ds) != 2 {
		t.Fatalf("decisions: %v", ds)
	}
	d0, d1 := obj(ds[0]), obj(ds[1])
	if str(d0["resolution_status"]) != "DISPUTED" || str(d0["note"]) != "tedarikçi iade edecek" || str(d0["decided_by_email"]) != email || str(d0["decided_at"]) == "" {
		t.Errorf("first decision: %v", d0)
	}
	if str(d1["resolution_status"]) != "OPEN" || d1["note"] != nil || d1["decided_by_email"] != nil {
		t.Errorf("second decision: %v", d1)
	}

	// append-only: the app role may not rewrite the trail
	for _, q := range []string{"UPDATE finding_decisions SET note='x'", "DELETE FROM finding_decisions"} {
		err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
			_, err := tx.Exec(context.Background(), q)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s: %v", q, err)
		}
	}
}
