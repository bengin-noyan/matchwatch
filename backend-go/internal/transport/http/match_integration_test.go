package transporthttp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

func (e *env) id(q string, args ...any) uuid.UUID {
	e.t.Helper()
	var id uuid.UUID
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), q, args...).Scan(&id)
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestLineMatchCreatesAliasAndReaudits(t *testing.T) {
	e := setup(t, "9876543210")
	e.loadContract()
	code, b := e.req("POST", "/api/v1/contracts", fixture(t, "contract_basic.json",
		`"vendor_vkn": "1234567890"`, `"vendor_vkn": "1111111111"`, "S-2026-001", "S-OTHER-"+uuid.NewString()[:8]))
	e.mustStatus(201, code, b)
	code, b = e.req("POST", "/api/v1/despatches", despatchXML(t))
	e.mustStatus(201, code, b)
	code, b = e.req("POST", "/api/v1/invoices", fixture(t, "invoice_basic.xml", invETTN, uuid.NewString(),
		"ABC2026000000001", "T"+uuid.NewString()[:12],
		"<cac:SellersItemIdentification><cbc:ID>CV-M8-20</cbc:ID></cac:SellersItemIdentification>", "",
		`<cac:StandardItemIdentification><cbc:ID schemeID="GTIN">8690000000017</cbc:ID></cac:StandardItemIdentification>`, ""))
	e.mustStatus(201, code, b)
	invID := str(b["id"])
	line := e.id(`SELECT l.id FROM invoice_lines l WHERE l.invoice_id=$1 AND l.line_id='1'`, invID)
	item := e.id(`SELECT ci.id FROM contract_items ci JOIN contracts c ON c.id=ci.contract_id
		WHERE c.contract_no='S-2026-001' AND ci.item_code='CV-M8-20'`)
	other := e.id(`SELECT ci.id FROM contract_items ci JOIN contracts c ON c.id=ci.contract_id
		WHERE c.contract_no LIKE 'S-OTHER-%' AND ci.item_code='CV-M8-20'`)

	path := "/api/v1/invoice-lines/" + line.String() + "/match"
	for _, tc := range []struct {
		path, body string
		want       int
	}{
		{path, `{"contract_item_id":"` + other.String() + `"}`, 422},
		{path, `{"contract_item_id":"` + uuid.NewString() + `"}`, 404},
		{"/api/v1/invoice-lines/" + uuid.NewString() + "/match", `{"contract_item_id":"` + item.String() + `"}`, 404},
		{"/api/v1/invoice-lines/x/match", `{"contract_item_id":"` + item.String() + `"}`, 404},
		{path, `{"contract_item_id":"nope"}`, 400},
		{path, `[`, 400},
	} {
		if code, b := e.req("POST", tc.path, []byte(tc.body)); code != tc.want {
			t.Errorf("%s %s: %d %v", tc.path, tc.body, code, b)
		}
	}
	if code, _ := e.do("POST", path, "", []byte(`{"contract_item_id":"`+item.String()+`"}`)); code != 401 {
		t.Errorf("no token: %d", code)
	}

	code, b = e.req("POST", path, []byte(`{"contract_item_id":"`+item.String()+`"}`))
	e.mustStatus(200, code, b)
	if str(b["invoice_id"]) != invID || str(b["audit_status"]) != "RED" {
		t.Fatalf("body %v", b)
	}
	var n int
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM audit_findings
			WHERE invoice_id=$1 AND rule_code='PRICE_MISMATCH' AND evidence_json->>'match_method'='ALIAS'`, invID).Scan(&n)
	})
	if err != nil || n != 1 {
		t.Fatalf("alias price findings %d err %v", n, err)
	}
}
