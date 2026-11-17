package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// vec, verilen ilk bileşenlerle başlayan, gerisi sıfır olan 1024 boyutlu bir pgvector değeri üretir.
func vec(lead ...string) json.RawMessage {
	parts := make([]string, 1024)
	for i := range parts {
		parts[i] = "0"
		if i < len(lead) {
			parts[i] = lead[i]
		}
	}
	return json.RawMessage("[" + strings.Join(parts, ",") + "]")
}

// semanticContract t.Fatal çağırmamalı: bir tenant transaction'ının içinde çalışıyor.
func semanticContract(ctx context.Context, tx *Tx, vendor uuid.UUID) (uuid.UUID, map[string]uuid.UUID, error) {
	item := func(name, uom string) ContractItemInput {
		return ContractItemInput{Name: name, UOM: uom, Currency: "TRY", UnitPrice: decimal.RequireFromString("1"), RequiresDelivery: true}
	}
	cid, err := tx.InsertContract(ctx, ContractInput{
		VendorID: vendor, Number: "C-" + uuid.NewString(), PaymentTermDays: 30, Currency: "TRY", QtyTolerancePct: decimal.Zero,
		ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ValidUntil: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		Items: []ContractItemInput{item("exact", "C62"), item("near", "C62"), item("other uom", "KGM"), item("other model", "C62")},
	})
	if err != nil {
		return cid, nil, err
	}
	ids := map[string]uuid.UUID{}
	rows, err := tx.Query(ctx, `SELECT item_name, id FROM contract_items WHERE contract_id=$1`, cid)
	if err != nil {
		return cid, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		var id uuid.UUID
		if err := rows.Scan(&n, &id); err != nil {
			return cid, nil, err
		}
		ids[n] = id
	}
	return cid, ids, rows.Err()
}

func TestSuggestionsRankAndFilter(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		v, err := tx.UpsertVendor(ctx, "1111111111", "V")
		if err != nil {
			return err
		}
		cid, ids, err := semanticContract(ctx, tx, v)
		if err != nil {
			return err
		}
		need, err := tx.ItemsNeedingEmbedding(ctx, cid, "m1")
		if err != nil || len(need) != 4 {
			t.Errorf("need=%v err=%v", need, err)
			return errors.New("fail")
		}
		for name, v := range map[string]json.RawMessage{"exact": vec("1"), "near": vec("1", "1"), "other uom": vec("1")} {
			if err := tx.SetItemEmbedding(ctx, ids[name], "m1", v); err != nil {
				return err
			}
		}
		if err := tx.SetItemEmbedding(ctx, ids["other model"], "m2", vec("1")); err != nil {
			return err
		}
		need, err = tx.ItemsNeedingEmbedding(ctx, cid, "m1")
		if err != nil || len(need) != 1 || need[0].Name != "other model" {
			t.Errorf("need after=%v err=%v", need, err)
			return errors.New("fail")
		}
		got, err := tx.Suggestions(ctx, cid, domain.NormalizeUOM("C62"), "m1", vec("1"))
		if err != nil {
			return err
		}
		if len(got) != 2 || got[0].ItemName != "exact" || got[0].Similarity.String() != "1" ||
			got[1].ItemName != "near" || got[1].Similarity.String() != "0.7071" || got[0].ContractItemID != ids["exact"] {
			t.Errorf("suggestions=%+v", got)
			return errors.New("fail")
		}
		if err := tx.SetItemEmbedding(ctx, ids["exact"], "m1", vec("1", "2")[:20]); err == nil {
			t.Error("malformed vector accepted")
		}
		return errors.New("rollback")
	})
	if err == nil || err.Error() != "rollback" {
		t.Fatal(err)
	}
}

func TestUpsertAliasAndRLS(t *testing.T) {
	s, tid := setup(t)
	ctx := context.Background()
	inv := parseInvoice(t)
	var lineID, invID, itemID, otherItem uuid.UUID
	err := s.InTenant(ctx, tid, func(tx *Tx) error {
		v, err := tx.UpsertVendor(ctx, inv.Supplier.VKN, inv.Supplier.Title)
		if err != nil {
			return err
		}
		if invID, err = tx.InsertInvoice(ctx, v, inv, "raw/inv.xml"); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id FROM invoice_lines WHERE invoice_id=$1 AND line_id='1'`, invID).Scan(&lineID); err != nil {
			return err
		}
		_, ids, err := semanticContract(ctx, tx, v)
		if err != nil {
			return err
		}
		itemID = ids["exact"]
		other, err := tx.UpsertVendor(ctx, "2222222222", "Other")
		if err != nil {
			return err
		}
		_, oids, err := semanticContract(ctx, tx, other)
		if err != nil {
			return err
		}
		otherItem = oids["exact"]

		if _, err := tx.UpsertAlias(ctx, lineID, otherItem); !errors.Is(err, ErrVendorMismatch) {
			t.Errorf("other vendor: %v", err)
		}
		if _, err := tx.UpsertAlias(ctx, uuid.New(), itemID); !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown line: %v", err)
		}
		if _, err := tx.UpsertAlias(ctx, lineID, uuid.New()); !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown item: %v", err)
		}
		got, err := tx.UpsertAlias(ctx, lineID, ids["near"])
		if err != nil || got != invID {
			t.Errorf("upsert: %v %v", got, err)
			return errors.New("fail")
		}
		if _, err := tx.UpsertAlias(ctx, lineID, itemID); err != nil { // ikinci seçim ilkinin yerine geçer
			return err
		}
		al, err := tx.Aliases(ctx, v)
		if err != nil {
			return err
		}
		if len(al) != 1 || al[domainKey("M8 20mm Inox Civata", domain.NormalizeUOM("C62"))] != itemID {
			t.Errorf("aliases=%v", al)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateTenant(ctx, "T2", uuid.NewString()[:8]+"01", decimal.RequireFromString("0.45"))
	cleanupTenant(t, other)
	err = s.InTenant(ctx, other, func(tx *Tx) error {
		if _, err := tx.UpsertAlias(ctx, lineID, itemID); !errors.Is(err, ErrNotFound) {
			t.Errorf("cross-tenant upsert: %v", err)
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM item_aliases`).Scan(&n); err != nil || n != 0 {
			t.Errorf("cross-tenant aliases visible: %d %v", n, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func domainKey(name, uom string) domain.AliasKey {
	return domain.AliasKey{NameKey: domain.NameKey(name), UOM: uom}
}
