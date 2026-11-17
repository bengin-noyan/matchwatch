package transporthttp

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFindingListAndBulkDecisions(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	run := strings.ToUpper(uuid.NewString()[:6])
	invA := e.postInvoiceAs("A"+run+"0000001", "2026-03-01", false)
	invB := e.postInvoiceAs("B"+run+"0000002", "2026-04-15", false)
	findings := func(inv string) []map[string]any {
		t.Helper()
		code, b := e.req("GET", "/api/v1/invoices/"+inv, nil)
		e.mustStatus(200, code, b)
		var out []map[string]any
		for _, f := range list(b["findings"]) {
			out = append(out, obj(f))
		}
		return out
	}
	fa, fb := findings(invA), findings(invB)
	if len(fa) < 2 || len(fb) < 2 {
		t.Fatalf("fixture should yield several findings per invoice: %d, %d", len(fa), len(fb))
	}

	// liste: tenant'ın bütün OPEN bulguları, etkisi en büyük olan önce, kural başına sayılmış
	code, b := e.req("GET", "/api/v1/findings?resolution=OPEN", nil)
	e.mustStatus(200, code, b)
	if n, _ := strconv.Atoi(num(b["total"])); n != len(fa)+len(fb) {
		t.Fatalf("total %d, want %d", n, len(fa)+len(fb))
	}
	sum, last := 0, int64(1<<62)
	for _, v := range obj(b["counts"]) {
		n, _ := strconv.Atoi(num(v))
		sum += n
	}
	if sum != len(fa)+len(fb) {
		t.Fatalf("counts sum %d", sum)
	}
	for _, it := range list(b["items"]) {
		imp, _ := strconv.ParseInt(num(obj(it)["financial_impact_minor"]), 10, 64)
		if imp > last {
			t.Fatalf("not ordered by impact: %v", list(b["items"]))
		}
		last = imp
	}
	rule := str(fa[0]["rule_code"])
	code, b = e.req("GET", "/api/v1/findings?resolution=OPEN&rule="+rule, nil)
	e.mustStatus(200, code, b)
	if num(b["total"]) != num(obj(b["counts"])[rule]) {
		t.Fatalf("rule filter total %v, counts %v", b["total"], b["counts"])
	}
	for _, it := range list(b["items"]) {
		if str(obj(it)["rule_code"]) != rule {
			t.Fatalf("rule filter leaked %v", it)
		}
	}

	// bilinmeyen bir id bütün grubu düşürür: hiçbirine karar verilmez
	body := func(status string, ids ...string) []byte {
		return []byte(`{"resolution_status":"` + status + `","note":"toplu onay","finding_ids":["` + strings.Join(ids, `","`) + `"]}`)
	}
	code, b = e.req("POST", "/api/v1/findings/decisions", body("APPROVED_EXCEPTION", str(fa[0]["id"]), uuid.NewString()))
	e.mustStatus(404, code, b)
	for _, f := range findings(invA) {
		if str(f["resolution_status"]) != "OPEN" || len(list(f["decisions"])) != 0 {
			t.Fatalf("failed batch changed %v", f)
		}
	}

	// A'nın hepsi ve B'den bir tane, bir de tekrar eden id: A GREEN olur, B'nin diğer bulguları açık kalır
	ids := []string{str(fb[0]["id"]), str(fa[0]["id"])}
	for _, f := range fa {
		ids = append(ids, str(f["id"]))
	}
	code, b = e.req("POST", "/api/v1/findings/decisions", body("APPROVED_EXCEPTION", ids...))
	e.mustStatus(200, code, b)
	if num(b["updated"]) != strconv.Itoa(len(fa)+1) {
		t.Fatalf("updated %v, want %d", b["updated"], len(fa)+1)
	}
	code, b = e.req("GET", "/api/v1/invoices/"+invA, nil)
	e.mustStatus(200, code, b)
	if str(b["audit_status"]) != "GREEN" {
		t.Fatalf("invoice A %s after approving all its findings", str(b["audit_status"]))
	}
	for _, f := range findings(invA) {
		ds := list(f["decisions"])
		if str(f["resolution_status"]) != "APPROVED_EXCEPTION" || len(ds) != 1 || str(obj(ds[0])["note"]) != "toplu onay" {
			t.Fatalf("finding after batch: %v", f)
		}
	}
	open := 0
	for _, f := range findings(invB) {
		if str(f["resolution_status"]) == "OPEN" {
			open++
		}
	}
	if open != len(fb)-1 {
		t.Fatalf("invoice B open findings %d, want %d", open, len(fb)-1)
	}
	code, b = e.req("GET", "/api/v1/findings?resolution=APPROVED_EXCEPTION", nil)
	e.mustStatus(200, code, b)
	if num(b["total"]) != strconv.Itoa(len(fa)+1) {
		t.Fatalf("approved total %v", b["total"])
	}

	// başka bir tenant bu bulgulara karar veremez
	other := setup(t, "9876543210")
	code, b = other.req("POST", "/api/v1/findings/decisions", body("DISPUTED", str(fb[1]["id"])))
	other.mustStatus(404, code, b)

	tooMany := make([]string, 201)
	for i := range tooMany {
		tooMany[i] = uuid.NewString()
	}
	for name, c := range map[string]struct {
		body []byte
		want int
	}{
		"no ids":       {[]byte(`{"resolution_status":"DISPUTED","finding_ids":[]}`), 422},
		"too many ids": {body("DISPUTED", tooMany...), 422},
		"bad status":   {body("FOO", str(fb[1]["id"])), 422},
		"bad id":       {[]byte(`{"resolution_status":"DISPUTED","finding_ids":["nope"]}`), 400},
	} {
		if code, b := e.req("POST", "/api/v1/findings/decisions", c.body); code != c.want {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
	for _, q := range []string{"resolution=FOO", "vendor_id=x", "limit=0", "offset=-1"} {
		if code, b := e.req("GET", "/api/v1/findings?"+q, nil); code != 400 {
			t.Errorf("%s: %d %v", q, code, b)
		}
	}
}
