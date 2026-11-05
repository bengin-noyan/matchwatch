package transporthttp

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

func TestAliasListAndDelete(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/despatches", despatchXML(t))
	e.mustStatus(201, code, b)
	// invoices whose line carries no item code or barcode: only an alias can match it
	post := func() string {
		code, b := e.req("POST", "/api/v1/invoices", fixture(t, "invoice_basic.xml", invETTN, uuid.NewString(),
			"ABC2026000000001", "T"+uuid.NewString()[:12],
			"<cac:SellersItemIdentification><cbc:ID>CV-M8-20</cbc:ID></cac:SellersItemIdentification>", "",
			`<cac:StandardItemIdentification><cbc:ID schemeID="GTIN">8690000000017</cbc:ID></cac:StandardItemIdentification>`, ""))
		e.mustStatus(201, code, b)
		return str(b["id"])
	}
	inv1 := post()
	line := e.id(`SELECT id FROM invoice_lines WHERE invoice_id=$1 AND line_id='1'`, inv1)
	item := e.id(`SELECT ci.id FROM contract_items ci JOIN contracts c ON c.id=ci.contract_id
		WHERE c.contract_no='S-2026-001' AND ci.item_code='CV-M8-20'`)
	code, b = e.req("POST", "/api/v1/invoice-lines/"+line.String()+"/match", []byte(`{"contract_item_id":"`+item.String()+`"}`))
	e.mustStatus(200, code, b)
	inv2 := post() // matched through the alias at ingest
	aliasFindings := func(inv string) (alias, uncontracted int) {
		t.Helper()
		err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE evidence_json->>'match_method'='ALIAS'),
				count(*) FILTER (WHERE rule_code='UNCONTRACTED_ITEM') FROM audit_findings WHERE invoice_id=$1`, inv).Scan(&alias, &uncontracted)
		})
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, inv := range []string{inv1, inv2} {
		if a, u := aliasFindings(inv); a == 0 || u != 0 {
			t.Fatalf("%s before delete: %d alias, %d uncontracted findings", inv, a, u)
		}
	}

	code, b = e.req("GET", "/api/v1/aliases", nil)
	e.mustStatus(200, code, b)
	items := list(b["items"])
	if len(items) != 1 {
		t.Fatalf("aliases %v", items)
	}
	a := obj(items[0])
	ci := obj(a["contract_item"])
	if str(a["name_key"]) != "m8 20mm inox civata" || str(a["vendor_name"]) != "Demir Bağlantı A.Ş." ||
		str(ci["id"]) != item.String() || str(ci["contract_no"]) != "S-2026-001" || str(a["created_at"]) == "" {
		t.Fatalf("alias %v", a)
	}
	aliasID := str(a["id"])
	code, b = e.req("GET", "/api/v1/aliases?vendor_id="+uuid.NewString(), nil)
	e.mustStatus(200, code, b)
	if len(list(b["items"])) != 0 {
		t.Fatalf("vendor filter leaked %v", b)
	}
	if code, _ := e.req("GET", "/api/v1/aliases?vendor_id=x", nil); code != 400 {
		t.Errorf("bad vendor_id: %d", code)
	}

	other := setup(t, "9876543210")
	code, b = other.req("DELETE", "/api/v1/aliases/"+aliasID, nil)
	other.mustStatus(404, code, b)

	code, b = e.req("DELETE", "/api/v1/aliases/"+aliasID, nil)
	e.mustStatus(200, code, b)
	var got []string
	for _, v := range list(b["reaudited"]) {
		got = append(got, str(v))
	}
	slices.Sort(got)
	want := []string{inv1, inv2}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("reaudited %v, want %v", got, want)
	}
	for _, inv := range want {
		if a, u := aliasFindings(inv); a != 0 || u != 1 {
			t.Fatalf("%s after delete: %d alias, %d uncontracted findings", inv, a, u)
		}
	}
	code, b = e.req("GET", "/api/v1/aliases", nil)
	e.mustStatus(200, code, b)
	if len(list(b["items"])) != 0 {
		t.Fatalf("alias still listed %v", b)
	}
	code, b = e.req("DELETE", "/api/v1/aliases/"+aliasID, nil)
	e.mustStatus(404, code, b)
}
