-- Bir kalem eşleşmesini kimin onayladığı ve kimin kaldırdığı; yalnızca eklenir, değiştirilmez.
-- Eşleşme silinince satırı da gider, bu yüzden kayıt eşleşmeye bağlı değil: ne olduğunun bir kopyasını (tedarikçi,
-- faturadaki ad, birim, sözleşme kalemi) kendisi taşır.
-- Geriye dönük doldurma yok: bu migration'dan önceki eşleşmeleri kimin onayladığı bilinmiyor.
CREATE TABLE alias_events (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id),
  alias_id      UUID NOT NULL,
  contract_item_id UUID NOT NULL,
  action        TEXT NOT NULL CHECK (action IN ('CONFIRMED','REMOVED')),
  vendor_id     UUID NOT NULL,
  name_key      TEXT NOT NULL,
  uom           TEXT NOT NULL,
  contract_no   TEXT NOT NULL,
  item_name     TEXT NOT NULL,
  -- users.id için FK yok: kayıt, kullanıcı silinse de kalmalı. E-posta o anki bir kopyadır; dev token'larda ikisi de NULL.
  by_user       UUID,
  by_email      TEXT,
  at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, vendor_id) REFERENCES vendors(tenant_id, id)
);
CREATE INDEX alias_events_alias_idx ON alias_events (tenant_id, alias_id, at);
CREATE INDEX alias_events_removed_idx ON alias_events (tenant_id, at) WHERE action = 'REMOVED';

ALTER TABLE alias_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE alias_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON alias_events
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Uygulama için yalnızca ekleme: UPDATE ya da DELETE yok.
GRANT SELECT, INSERT ON alias_events TO matchwatch_app;
