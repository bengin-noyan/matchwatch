package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

var (
	ErrVendorMismatch = errors.New("contract item belongs to another vendor")
	ErrEmptyItemName  = errors.New("invoice line has no usable item name")
)

// Vektörler JSON metni olarak taşınır (geçerli bir pgvector değeri) ve SQL'de çevrilir; böylece Go'ya hiç float girmez.

func (tx *Tx) Aliases(ctx context.Context, vendorID uuid.UUID) (map[domain.AliasKey]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT name_key, uom, contract_item_id FROM item_aliases WHERE tenant_id=$1 AND vendor_id=$2`,
		tx.tenantID, vendorID)
	if err != nil {
		return nil, wrap("aliases", err)
	}
	defer rows.Close()
	out := map[domain.AliasKey]uuid.UUID{}
	for rows.Next() {
		var k domain.AliasKey
		var id uuid.UUID
		if err := rows.Scan(&k.NameKey, &k.UOM, &id); err != nil {
			return nil, wrap("scan alias", err)
		}
		out[k] = id
	}
	return out, rows.Err()
}

// ItemsNeedingEmbedding, sözleşmenin bu model için vektörü olmayan kalemlerini listeler (yalnızca ID ve ad).
func (tx *Tx) ItemsNeedingEmbedding(ctx context.Context, contractID uuid.UUID, model string) ([]domain.ContractItem, error) {
	rows, err := tx.Query(ctx, `SELECT id, item_name FROM contract_items
		WHERE tenant_id=$1 AND contract_id=$2 AND embedding_model IS DISTINCT FROM $3 ORDER BY id`, tx.tenantID, contractID, model)
	if err != nil {
		return nil, wrap("items needing embedding", err)
	}
	defer rows.Close()
	var out []domain.ContractItem
	for rows.Next() {
		var it domain.ContractItem
		if err := rows.Scan(&it.ID, &it.Name); err != nil {
			return nil, wrap("scan item", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (tx *Tx) SetItemEmbedding(ctx context.Context, itemID uuid.UUID, model string, vec json.RawMessage) error {
	tag, err := tx.Exec(ctx, `UPDATE contract_items SET name_embedding=$3::text::vector, embedding_model=$4
		WHERE tenant_id=$1 AND id=$2`, tx.tenantID, itemID, string(vec), model)
	if err != nil {
		return wrap("set item embedding", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Suggestions, sözleşmenin aynı birimdeki ve bu modelle embed edilmiş kalemlerini vec'e kosinüs benzerliğine göre
// sıralar (en iyi 3).
func (tx *Tx) Suggestions(ctx context.Context, contractID uuid.UUID, uom, model string, vec json.RawMessage) ([]domain.Suggestion, error) {
	rows, err := tx.Query(ctx, `SELECT id, item_name, round((1 - (name_embedding <=> $4::text::vector))::numeric, 4)::text
		FROM contract_items WHERE tenant_id=$1 AND contract_id=$2 AND uom=$3 AND embedding_model=$5
		ORDER BY name_embedding <=> $4::text::vector, id LIMIT 3`, tx.tenantID, contractID, uom, string(vec), model)
	if err != nil {
		return nil, wrap("suggestions", err)
	}
	defer rows.Close()
	var out []domain.Suggestion
	for rows.Next() {
		var s domain.Suggestion
		var sim string
		if err := rows.Scan(&s.ContractItemID, &s.ItemName, &sim); err != nil {
			return nil, wrap("scan suggestion", err)
		}
		s.Similarity = dec(sim)
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpsertAlias, lineID'deki kalem adının (o tedarikçi ve birim için) itemID anlamına geldiğini kaydeder; önceki seçimin
// yerine geçer. Satırın faturasını döner ki çağıran onu yeniden denetleyebilsin.
func (tx *Tx) UpsertAlias(ctx context.Context, lineID, itemID uuid.UUID) (uuid.UUID, error) {
	var invoiceID, vendorID, itemVendor uuid.UUID
	var name, uom string
	err := tx.QueryRow(ctx, `SELECT il.invoice_id, i.vendor_id, il.item_name, il.uom FROM invoice_lines il
		JOIN invoices i ON i.tenant_id=il.tenant_id AND i.id=il.invoice_id WHERE il.tenant_id=$1 AND il.id=$2`,
		tx.tenantID, lineID).Scan(&invoiceID, &vendorID, &name, &uom)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, wrap("alias line", err)
	}
	err = tx.QueryRow(ctx, `SELECT c.vendor_id FROM contract_items ci
		JOIN contracts c ON c.tenant_id=ci.tenant_id AND c.id=ci.contract_id WHERE ci.tenant_id=$1 AND ci.id=$2`,
		tx.tenantID, itemID).Scan(&itemVendor)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, wrap("alias item", err)
	}
	if itemVendor != vendorID {
		return uuid.Nil, ErrVendorMismatch
	}
	key := domain.NameKey(name)
	if key == "" {
		return uuid.Nil, ErrEmptyItemName
	}
	_, err = tx.Exec(ctx, `INSERT INTO item_aliases(tenant_id, vendor_id, name_key, uom, contract_item_id) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id, vendor_id, name_key, uom) DO UPDATE SET contract_item_id=EXCLUDED.contract_item_id, created_at=now()`,
		tx.tenantID, vendorID, key, uom, itemID)
	return invoiceID, wrap("upsert alias", err)
}

// AliasRow, bir insanın onayladığı tek bir kalem eşleşmesi ve gösterdiği sözleşme kalemidir.
type AliasRow struct {
	ID           uuid.UUID       `json:"id"`
	VendorID     uuid.UUID       `json:"vendor_id"`
	VendorName   string          `json:"vendor_name"`
	NameKey      string          `json:"name_key"` // faturadaki kalem adı, normalleştirilmiş (domain.NameKey)
	UOM          string          `json:"uom"`
	ContractItem ContractItemRow `json:"contract_item"`
	CreatedAt    time.Time       `json:"created_at"` // onaylandığı (ya da en son değiştirildiği) zaman
}

// ListAliases, tenant'ın eşleşmelerini (istenirse tek bir tedarikçininkileri) tedarikçi ve ada göre sıralı döner.
// shortcut: sayfalama yok; bir tenant'ta on binlerce değil yüzlerce eşleşme olmasını bekliyoruz.
func (tx *Tx) ListAliases(ctx context.Context, vendorID *uuid.UUID) ([]AliasRow, error) {
	rows, err := tx.Query(ctx, `SELECT a.id, a.vendor_id, v.name, a.name_key, a.uom, a.created_at,
		ci.id, c.contract_no, ci.item_code, ci.item_name, ci.unit_price::text, ci.currency, ci.uom, ci.source_page, ci.source_clause_text
		FROM item_aliases a
		JOIN vendors v ON v.tenant_id=a.tenant_id AND v.id=a.vendor_id
		JOIN contract_items ci ON ci.tenant_id=a.tenant_id AND ci.id=a.contract_item_id
		JOIN contracts c ON c.tenant_id=ci.tenant_id AND c.id=ci.contract_id
		WHERE a.tenant_id=$1 AND ($2::uuid IS NULL OR a.vendor_id=$2) ORDER BY v.name, a.name_key, a.uom`, tx.tenantID, vendorID)
	if err != nil {
		return nil, wrap("list aliases", err)
	}
	defer rows.Close()
	out := []AliasRow{}
	for rows.Next() {
		var a AliasRow
		ci := &a.ContractItem
		if err := rows.Scan(&a.ID, &a.VendorID, &a.VendorName, &a.NameKey, &a.UOM, &a.CreatedAt,
			&ci.ID, &ci.ContractNo, &ci.ItemCode, &ci.Name, &ci.UnitPrice, &ci.Currency, &ci.UOM, &ci.SourcePage, &ci.SourceClauseText); err != nil {
			return nil, wrap("scan alias", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAlias bir eşleşmeyi siler ve bir satırı bu eşleşmeyle eşleşmiş faturaları döner, ki çağıran onları yeniden
// denetleyebilsin (bu satırlar yeni önerilerle UNCONTRACTED_ITEM'a döner).
func (tx *Tx) DeleteAlias(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	var vendorID, itemID uuid.UUID
	var key, uom string
	err := tx.QueryRow(ctx, `DELETE FROM item_aliases WHERE tenant_id=$1 AND id=$2 RETURNING vendor_id, name_key, uom, contract_item_id`,
		tx.tenantID, id).Scan(&vendorID, &key, &uom, &itemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, wrap("delete alias", err)
	}
	// NameKey Go tarafında hesaplanıyor; o yüzden SQL'de kalem ve birime göre daraltıp anahtarları burada
	// karşılaştırıyoruz.
	rows, err := tx.Query(ctx, `SELECT il.invoice_id, il.item_name FROM invoice_lines il
		JOIN invoices i ON i.tenant_id=il.tenant_id AND i.id=il.invoice_id
		WHERE il.tenant_id=$1 AND i.vendor_id=$2 AND il.matched_contract_item_id=$3 AND il.uom=$4`, tx.tenantID, vendorID, itemID, uom)
	if err != nil {
		return nil, wrap("alias invoices", err)
	}
	defer rows.Close()
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for rows.Next() {
		var inv uuid.UUID
		var name string
		if err := rows.Scan(&inv, &name); err != nil {
			return nil, wrap("scan alias invoice", err)
		}
		if domain.NameKey(name) == key && !seen[inv] {
			seen[inv] = true
			out = append(out, inv)
		}
	}
	return out, rows.Err()
}
