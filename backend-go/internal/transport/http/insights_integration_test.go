package transporthttp

import (
	"testing"

	"github.com/google/uuid"
)

func TestDashboardAndVendors(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/despatches", despatchXML(t))
	e.mustStatus(201, code, b)
	code, b = e.req("POST", "/api/v1/invoices", invoiceXML(t))
	e.mustStatus(201, code, b)
	invID := str(b["id"])

	code, d := e.req("GET", "/api/v1/dashboard", nil)
	e.mustStatus(200, code, d)
	cur := obj(list(d["by_currency"])[0])
	if str(cur["currency"]) != "TRY" || num(cur["caught_minor"]) != "14912" || num(cur["open_count"]) != "2" {
		t.Fatalf("by_currency %v", d["by_currency"])
	}
	if w := list(d["weekly"]); len(w) != 1 || str(obj(w[0])["week"]) != "2026-02-23" || num(obj(w[0])["caught_minor"]) != "14912" {
		t.Fatalf("weekly %v", w) // invoice of Sunday 2026-03-01 belongs to the week starting Monday 2026-02-23
	}
	rules := list(d["by_rule"])
	if len(rules) != 2 || str(obj(rules[0])["rule_code"]) != "MATURITY_VIOLATION" || num(obj(rules[0])["impact_minor"]) != "9912" {
		t.Fatalf("by_rule %v", rules)
	}
	if v := list(d["by_vendor"]); len(v) != 1 || str(obj(v[0])["vendor_name"]) != "Demir Bağlantı A.Ş." || num(obj(v[0])["count"]) != "2" {
		t.Fatalf("by_vendor %v", v)
	}
	open := list(d["open_top"])
	if len(open) != 2 || str(obj(open[0])["invoice_id"]) != invID || num(obj(open[0])["impact_minor"]) != "9912" || obj(open[0])["line_id"] != nil {
		t.Fatalf("open_top %v", open)
	}
	if num(obj(d["invoices"])["RED"]) != "1" {
		t.Fatalf("invoices %v", d["invoices"])
	}

	code, d = e.req("GET", "/api/v1/dashboard?from=2026-03-02&to=2026-12-31", nil)
	e.mustStatus(200, code, d)
	if len(list(d["by_currency"])) != 0 || len(list(d["open_top"])) != 0 || num(obj(d["invoices"])["RED"]) != "0" {
		t.Fatalf("filtered dashboard %v", d)
	}
	if code, _ := e.req("GET", "/api/v1/dashboard?from=01.03.2026", nil); code != 400 {
		t.Fatalf("bad date: %d", code)
	}

	code, l := e.req("GET", "/api/v1/vendors", nil)
	e.mustStatus(200, code, l)
	vs := list(l["items"])
	if len(vs) != 1 {
		t.Fatalf("vendors %v", vs)
	}
	v := obj(vs[0])
	caught := list(v["caught"])
	if str(v["contract_no"]) != "S-2026-001" || num(v["invoice_count"]) != "1" || num(v["open_findings"]) != "2" ||
		len(caught) != 1 || num(obj(caught[0])["caught_minor"]) != "14912" {
		t.Fatalf("vendor row %v", v)
	}

	code, vd := e.req("GET", "/api/v1/vendors/"+str(v["id"]), nil)
	e.mustStatus(200, code, vd)
	cs := list(vd["contracts"])
	if len(cs) != 1 || len(list(obj(cs[0])["items"])) != 2 || str(obj(cs[0])["status"]) != "ACTIVE" {
		t.Fatalf("contracts %v", cs)
	}
	var boltID string
	for _, it := range list(obj(cs[0])["items"]) {
		if str(obj(it)["item_code"]) == "CV-M8-20" {
			boltID = str(obj(it)["id"])
		}
	}
	ph := list(obj(vd["price_history"])[boltID])
	if len(ph) != 1 || str(obj(ph[0])["net_unit_price"]) != "1.300000" || str(obj(ph[0])["invoice_id"]) != invID {
		t.Fatalf("price history %v", vd["price_history"])
	}
	if code, _ := e.req("GET", "/api/v1/vendors/"+uuid.NewString(), nil); code != 404 {
		t.Fatalf("unknown vendor: %d", code)
	}
}
