package transporthttp

import (
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// postInvoiceAs posts invoice_basic.xml as invoice no, issued on date, optionally by another vendor.
func (e *env) postInvoiceAs(no, date string, otherVendor bool) {
	e.t.Helper()
	repl := []string{invETTN, uuid.NewString(), "ABC2026000000001", no, "<cbc:IssueDate>2026-03-01</cbc:IssueDate>", "<cbc:IssueDate>" + date + "</cbc:IssueDate>"}
	if otherVendor {
		repl = append(repl, `<cbc:ID schemeID="VKN">1234567890</cbc:ID>`, `<cbc:ID schemeID="VKN">1111111111</cbc:ID>`,
			"Demir Bağlantı A.Ş.", "Işık Lojistik A.Ş.")
	}
	code, b := e.req("POST", "/api/v1/invoices", fixture(e.t, "invoice_basic.xml", repl...))
	e.mustStatus(201, code, b)
}

func (e *env) invoiceNos(query string) ([]string, resp) {
	e.t.Helper()
	code, b := e.req("GET", "/api/v1/invoices?"+query, nil)
	e.mustStatus(200, code, b)
	var nos []string
	for _, it := range list(b["items"]) {
		nos = append(nos, str(obj(it)["invoice_no"]))
	}
	return nos, b
}

func TestInvoiceListSearchFiltersPaging(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	run := strings.ToUpper(uuid.NewString()[:6])
	a, b, c := "A"+run+"0000001", "B"+run+"0000002", "C"+run+"0000003"
	e.postInvoiceAs(a, "2026-03-01", false)
	e.postInvoiceAs(b, "2026-04-15", true)
	e.postInvoiceAs(c, "2026-05-20", false)

	check := func(query string, want ...string) resp {
		t.Helper()
		got, body := e.invoiceNos(query)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: got %v want %v", query, got, want)
		}
		return body
	}
	check("sort=issue_date", a, b, c)
	check("sort=issue_date&dir=desc", c, b, a)
	check("sort=invoice_no&dir=desc", c, b, a)
	// case-blind, Turkish i's folded, LIKE wildcards literal
	check("sort=issue_date&q=%C4%B1%C5%9F%C4%B1k", b)     // ışık
	check("sort=issue_date&q=I%C5%9EIK", b)               // IŞIK
	check("sort=issue_date&q=BA%C4%9ELANTI", a, c)        // BAĞLANTI
	check("sort=issue_date&q="+strings.ToLower(c[:4]), c) // invoice no
	check("sort=issue_date&q=%25")                        // a literal %
	check("sort=issue_date&from=2026-04-01", b, c)
	check("sort=issue_date&from=2026-04-01&to=2026-04-15", b)

	body := check("sort=issue_date&limit=1&offset=1", b)
	if num(body["total"]) != "3" {
		t.Fatalf("paged total %v", body)
	}
	sum := 0
	for st, n := range obj(body["counts"]) {
		k, _ := strconv.Atoi(num(n))
		sum += k
		// the status filter narrows items and total but not counts
		if _, sb := e.invoiceNos("status=" + st); num(sb["total"]) != num(n) || num(obj(sb["counts"])[st]) != num(n) {
			t.Fatalf("status %s: %v vs counts %v", st, sb, body["counts"])
		}
	}
	if sum != 3 {
		t.Fatalf("counts %v", body["counts"])
	}

	_, open := e.invoiceNos("open=true")
	for _, it := range list(open["items"]) {
		if num(obj(it)["open_findings"]) == "0" {
			t.Fatalf("open=true returned %v", it)
		}
	}

	for _, q := range []string{"from=2026-13-01", "to=yesterday", "open=yes", "sort=ettn", "dir=up", "q=" + strings.Repeat("x", 101)} {
		if code, b := e.req("GET", "/api/v1/invoices?"+q, nil); code != 400 {
			t.Fatalf("%s: %d %v", q, code, b)
		}
	}
}
