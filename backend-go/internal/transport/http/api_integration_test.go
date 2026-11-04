package transporthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/migrate"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

var secret = []byte(strings.Repeat("k", 32))

type env struct {
	t       *testing.T
	store   *repository.Store
	srv     http.Handler
	tid     uuid.UUID
	token   string
	storage string
}

func setup(t *testing.T, tenantVKN string) *env {
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
	e := &env{t: t, store: s, storage: t.TempDir()}
	if e.tid, err = s.CreateTenant(ctx, "api-"+uuid.NewString(), tenantVKN, decimal.RequireFromString("0.4500")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		tx, err := owner.Begin(ctx)
		if err != nil {
			return
		}
		defer tx.Rollback(ctx)
		tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", e.tid.String())
		for _, tbl := range []string{"invoices", "despatch_advices", "contract_drafts", "contracts", "vendors"} {
			tx.Exec(ctx, "DELETE FROM "+tbl+" WHERE tenant_id=$1", e.tid)
		}
		tx.Exec(ctx, "DELETE FROM tenants WHERE id=$1", e.tid)
		tx.Commit(ctx)
	})
	e.srv = NewRouter(s, audit.New(s, nil, nil), e.storage, secret, nil, 0)
	tok, err := auth.Sign(secret, e.tid, "test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	e.token = tok
	return e
}

type resp map[string]any

func (e *env) do(method, path, token string, body []byte) (int, resp) {
	e.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	var out resp
	d := json.NewDecoder(rec.Body)
	d.UseNumber() // no float64 anywhere, even in tests
	d.Decode(&out)
	return rec.Code, out
}

func (e *env) req(method, path string, body []byte) (int, resp) {
	e.t.Helper()
	return e.do(method, path, e.token, body)
}

func fixture(t *testing.T, name string, repl ...string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for i := 0; i < len(repl); i += 2 {
		if !strings.Contains(s, repl[i]) {
			t.Fatalf("%s lacks %q", name, repl[i])
		}
		s = strings.ReplaceAll(s, repl[i], repl[i+1])
	}
	return []byte(s)
}

const (
	invETTN = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	desETTN = "0b0c5c4e-6d43-4b5a-9f39-3c1f0c0e9a11"
)

func invoiceXML(t *testing.T) []byte {
	return fixture(t, "invoice_basic.xml", invETTN, uuid.NewString(), "ABC2026000000001", "T"+uuid.NewString()[:12])
}

func despatchXML(t *testing.T) []byte {
	return fixture(t, "despatch_basic.xml", desETTN, uuid.NewString())
}

func (e *env) mustStatus(want int, code int, body resp) {
	e.t.Helper()
	if code != want {
		e.t.Fatalf("status %d want %d body %v", code, want, body)
	}
}

func (e *env) loadContract() {
	e.t.Helper()
	code, b := e.req("POST", "/api/v1/contracts", fixture(e.t, "contract_basic.json"))
	e.mustStatus(201, code, b)
}

func (e *env) invoiceCount() int {
	var n int
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM invoices`).Scan(&n)
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func num(v any) string {
	n, _ := v.(json.Number)
	return n.String()
}

func TestFlowHappyPath(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/despatches", despatchXML(t))
	e.mustStatus(201, code, b)
	if str(b["id"]) == "" || len(list(b["reaudited"])) != 0 {
		t.Fatalf("despatch resp %v", b)
	}
	code, b = e.req("POST", "/api/v1/receipts", fixture(t, "receipt_basic.xml"))
	e.mustStatus(200, code, b)
	if str(b["despatch_no"]) != "IRS2026000000001" {
		t.Fatalf("receipt resp %v", b)
	}
	code, b = e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(201, code, b)
	if str(b["audit_status"]) != "RED" {
		t.Fatalf("invoice resp %v", b)
	}
	id := str(b["id"])
	for _, kind := range []string{"despatch", "invoice"} {
		if ents, _ := os.ReadDir(filepath.Join(e.storage, e.tid.String(), kind)); len(ents) != 1 {
			t.Fatalf("raw %s files: %v", kind, ents)
		}
	}

	code, b = e.req("GET", "/api/v1/invoices/"+id, nil)
	e.mustStatus(200, code, b)
	if n := len(list(b["findings"])); n != 3 {
		t.Fatalf("findings: %d", n)
	}
	line1 := obj(list(b["lines"])[0])
	if str(line1["line_id"]) != "1" || num(obj(line1["contract_item"])["source_page"]) != "3" {
		t.Fatalf("line 1: %v", line1)
	}
	if len(list(line1["despatch_lines"])) != 1 {
		t.Fatalf("despatch_lines: %v", line1["despatch_lines"])
	}

	code, b = e.req("GET", "/api/v1/invoices?status=RED&limit=10", nil)
	e.mustStatus(200, code, b)
	if num(b["total"]) != "1" || len(list(b["items"])) != 1 {
		t.Fatalf("list %v", b)
	}

	code, b = e.req("POST", "/api/v1/invoices/"+id+"/audit", nil)
	e.mustStatus(200, code, b)
	if str(b["audit_status"]) != "RED" {
		t.Fatalf("re-audit %v", b)
	}

	code, b = e.req("GET", "/api/v1/summary", nil)
	e.mustStatus(200, code, b)
	cur := obj(list(b["by_currency"])[0])
	if str(cur["currency"]) != "TRY" || num(cur["caught_minor"]) != "21412" { // 5000+6500+9912
		t.Fatalf("summary %v", b)
	}
	if num(obj(b["invoices"])["RED"]) != "1" {
		t.Fatalf("summary invoices %v", b["invoices"])
	}
}

func TestInvoiceDuplicateETTN(t *testing.T) {
	e := setup(t, "9876543210")
	x := invoiceXML(t)
	code, b := e.req("POST", "/api/v1/invoices", x)
	e.mustStatus(201, code, b)
	code, b = e.req("POST", "/api/v1/invoices", x)
	e.mustStatus(409, code, b)
	if str(b["code"]) != "DUPLICATE_ETTN" {
		t.Fatalf("%v", b)
	}
}

func TestInvoiceCustomerMismatch(t *testing.T) {
	e := setup(t, "1111111111")
	code, b := e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(422, code, b)
	if str(b["code"]) != "CUSTOMER_MISMATCH" {
		t.Fatalf("%v", b)
	}
	if n := e.invoiceCount(); n != 0 {
		t.Fatalf("invoices: %d", n)
	}
	if ents, _ := os.ReadDir(e.storage); len(ents) != 0 {
		t.Fatalf("files written: %v", ents)
	}
}

func TestInvoiceGarbageXML(t *testing.T) {
	e := setup(t, "9876543210")
	for _, body := range []string{"not xml at all", "<Invoice", string(fixture(t, "despatch_basic.xml"))} {
		code, b := e.req("POST", "/api/v1/invoices", []byte(body))
		e.mustStatus(422, code, b)
	}
}

func TestInvoiceBeforeDespatch(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(201, code, b)
	id := str(b["id"])
	noDespatch := func() bool {
		code, b := e.req("GET", "/api/v1/invoices/"+id, nil)
		e.mustStatus(200, code, b)
		for _, f := range list(b["findings"]) {
			if obj(obj(f)["evidence"])["no_despatch_found"] == true {
				return true
			}
		}
		return false
	}
	if str(b["audit_status"]) != "RED" || !noDespatch() {
		t.Fatalf("expected RED with no_despatch_found: %v", b)
	}
	code, b = e.req("POST", "/api/v1/despatches", despatchXML(t))
	e.mustStatus(201, code, b)
	re := list(b["reaudited"])
	if len(re) != 1 || re[0] != id {
		t.Fatalf("reaudited %v want [%s]", re, id)
	}
	if noDespatch() {
		t.Fatal("no_despatch_found still present after despatch")
	}
}

func TestCrossTenantInvoiceIs404(t *testing.T) {
	a := setup(t, "9876543210")
	b := setup(t, "9876543210")
	code, body := a.req("POST", "/api/v1/invoices", invoiceXML(t))
	a.mustStatus(201, code, body)
	id := str(body["id"])
	for _, c := range [][2]string{{"GET", ""}, {"POST", "/audit"}} {
		if code, body := b.req(c[0], "/api/v1/invoices/"+id+c[1], nil); code != 404 {
			t.Fatalf("%s: %d %v", c[0], code, body)
		}
	}
	if code, _ := a.req("GET", "/api/v1/invoices/"+id, nil); code != 200 {
		t.Fatalf("owner lost access: %d", code)
	}
}

func TestPatchFindingValidation(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(201, code, b)
	code, b = e.req("GET", "/api/v1/invoices/"+str(b["id"]), nil)
	e.mustStatus(200, code, b)
	fid := str(obj(list(b["findings"])[0])["id"])

	code, b = e.req("PATCH", "/api/v1/findings/"+fid, []byte(`{"resolution_status":"FOO"}`))
	e.mustStatus(422, code, b)
	code, b = e.req("PATCH", "/api/v1/findings/"+fid, []byte(`{"resolution_status":"DISPUTED","note":"vendor will credit"}`))
	e.mustStatus(200, code, b)
	code, b = e.req("PATCH", "/api/v1/findings/"+uuid.NewString(), []byte(`{"resolution_status":"DISPUTED"}`))
	e.mustStatus(404, code, b)
}

func TestAuthAndInputGuards(t *testing.T) {
	e := setup(t, "9876543210")
	if code, _ := e.do("GET", "/api/v1/summary", "", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := e.do("GET", "/healthz", "", nil); code != 200 {
		t.Fatalf("healthz: %d", code)
	}
	for _, q := range []string{"limit=abc", "limit=201", "limit=0", "offset=-1", "vendor_id=nope", "status=BOGUS"} {
		if code, b := e.req("GET", "/api/v1/invoices?"+q, nil); code != 400 {
			t.Fatalf("%s: %d %v", q, code, b)
		}
	}
	if code, b := e.req("GET", "/api/v1/invoices/not-a-uuid", nil); code != 404 {
		t.Fatalf("bad id: %d %v", code, b)
	}
	big := bytes.Repeat([]byte(" "), 10<<20+1)
	if code, b := e.req("POST", "/api/v1/invoices", big); code != 413 {
		t.Fatalf("oversize: %d %v", code, b)
	}
}

func TestContractValidation(t *testing.T) {
	e := setup(t, "9876543210")
	good := string(fixture(t, "contract_basic.json"))
	bad := map[string]string{
		"short vkn":        strings.Replace(good, `"1234567890"`, `"123"`, 1),
		"reversed dates":   strings.Replace(good, `"valid_until": "2026-12-31"`, `"valid_until": "2025-12-31"`, 1),
		"negative term":    strings.Replace(good, `"payment_term_days": 60`, `"payment_term_days": -1`, 1),
		"missing term":     strings.Replace(good, `"payment_term_days": 60,`, ``, 1),
		"tolerance 101":    strings.Replace(good, `"2.00"`, `"101"`, 1),
		"bad currency":     strings.Replace(good, `"currency": "TRY",`, `"currency": "try",`, 1),
		"missing currency": strings.Replace(good, `"currency": "TRY",`, ``, 1),
		"negative price":   strings.Replace(good, `"1.250000"`, `"-1"`, 1),
		"empty uom":        strings.Replace(good, `"uom": "C62",`, `"uom": "",`, 1),
		"no items":         `{"vendor_vkn":"1234567890","contract_no":"X","valid_from":"2026-01-01","valid_until":"2026-12-31","payment_term_days":1,"currency":"TRY","qty_tolerance_pct":"0","items":[]}`,
	}
	for name, body := range bad {
		if body == good {
			t.Fatalf("%s: replacement did not apply", name)
		}
		if code, b := e.req("POST", "/api/v1/contracts", []byte(body)); code != 422 {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
	// decimals as JSON numbers are accepted
	numeric := strings.Replace(good, `"1.250000"`, `1.25`, 1)
	if code, b := e.req("POST", "/api/v1/contracts", []byte(numeric)); code != 201 {
		t.Fatalf("numeric price: %d %v", code, b)
	}
	if code, b := e.req("POST", "/api/v1/contracts", []byte(good)); code != 409 {
		t.Fatalf("duplicate contract_no: %d %v", code, b)
	}
}

func TestSharedETTNKeepsBothRawFiles(t *testing.T) {
	e := setup(t, "9876543210")
	invXML := fixture(t, "invoice_basic.xml", invETTN, desETTN, "ABC2026000000001", "T"+uuid.NewString()[:12])
	desXML := fixture(t, "despatch_basic.xml") // keeps desETTN, which the invoice reuses
	for path, body := range map[string][]byte{"/api/v1/despatches": desXML, "/api/v1/invoices": invXML} {
		code, b := e.req("POST", path, body)
		e.mustStatus(201, code, b)
	}
	for kind, want := range map[string][]byte{"despatch": desXML, "invoice": invXML} {
		got, err := os.ReadFile(filepath.Join(e.storage, e.tid.String(), kind, desETTN+".xml"))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s raw file: err=%v, content matches=%v", kind, err, bytes.Equal(got, want))
		}
	}
}

func TestPatchFindingRefreshesAuditStatus(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(201, code, b)
	id := str(b["id"])
	code, b = e.req("GET", "/api/v1/invoices/"+id, nil)
	e.mustStatus(200, code, b)
	status := func() string {
		code, b := e.req("GET", "/api/v1/invoices/"+id, nil)
		e.mustStatus(200, code, b)
		return str(b["audit_status"])
	}
	if status() != "RED" {
		t.Fatalf("initial status %s", status())
	}
	// CRITICALs first: RED -> YELLOW only once both are approved, GREEN once the WARNING is too
	var crit, warn []string
	for _, f := range list(b["findings"]) {
		if str(obj(f)["severity"]) == "CRITICAL" {
			crit = append(crit, str(obj(f)["id"]))
		} else {
			warn = append(warn, str(obj(f)["id"]))
		}
	}
	approve := `{"resolution_status":"APPROVED_EXCEPTION"}`
	for i, fid := range append(crit, warn...) {
		code, b := e.req("PATCH", "/api/v1/findings/"+fid, []byte(approve))
		e.mustStatus(200, code, b)
		want := map[int]string{0: "RED", 1: "YELLOW", 2: "GREEN"}[i]
		if got := status(); got != want {
			t.Fatalf("after approving %d finding(s): %s, want %s", i+1, got, want)
		}
	}
	// reopening one brings the status back
	code, b = e.req("PATCH", "/api/v1/findings/"+crit[0], []byte(`{"resolution_status":"OPEN"}`))
	e.mustStatus(200, code, b)
	if got := status(); got != "RED" {
		t.Fatalf("after reopening: %s", got)
	}
}
