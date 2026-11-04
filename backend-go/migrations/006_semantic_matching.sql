-- Faz 2b (D16): contract item name vectors for suggestions, and human-confirmed item aliases.
-- vector(1024) fits bge-m3; a model with another dimension needs a new migration.
ALTER TABLE contract_items ADD COLUMN name_embedding vector(1024);
ALTER TABLE contract_items ADD COLUMN embedding_model TEXT;
ALTER TABLE contract_items ADD CONSTRAINT contract_items_embedding_pair
  CHECK ((name_embedding IS NULL) = (embedding_model IS NULL));

CREATE TABLE item_aliases (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id        UUID NOT NULL REFERENCES tenants(id),
  vendor_id        UUID NOT NULL,
  name_key         TEXT NOT NULL CHECK (name_key <> ''),
  uom              TEXT NOT NULL,
  contract_item_id UUID NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, vendor_id, name_key, uom),
  FOREIGN KEY (tenant_id, vendor_id) REFERENCES vendors(tenant_id, id),
  FOREIGN KEY (tenant_id, contract_item_id) REFERENCES contract_items(tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE item_aliases ENABLE ROW LEVEL SECURITY;
ALTER TABLE item_aliases FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON item_aliases
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON item_aliases TO matchwatch_app;
