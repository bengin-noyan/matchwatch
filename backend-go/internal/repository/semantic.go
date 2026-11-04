package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

var (
	ErrVendorMismatch = errors.New("contract item belongs to another vendor")
	ErrEmptyItemName  = errors.New("invoice line has no usable item name")
)

// Vectors travel as their JSON text (a valid pgvector literal) and are cast in SQL, so no float enters Go.

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

// ItemsNeedingEmbedding lists the contract's items with no vector for model (ID and Name only).
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

// Suggestions ranks the contract's same-uom items embedded with model by cosine similarity to vec (best 3).
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

// UpsertAlias records that lineID's item name (for its vendor and uom) means itemID, replacing an earlier choice.
// It returns the line's invoice so the caller can re-audit it.
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
