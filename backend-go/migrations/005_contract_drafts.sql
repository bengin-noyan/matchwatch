CREATE TABLE contract_drafts (
  id            UUID PRIMARY KEY,
  tenant_id     UUID NOT NULL REFERENCES tenants(id),
  status        TEXT NOT NULL CHECK (status IN ('EXTRACTING','READY','FAILED','APPROVED')),
  pdf_path      TEXT NOT NULL,
  pdf_sha256    TEXT NOT NULL,
  draft_json    JSONB,
  warnings_json JSONB NOT NULL DEFAULT '[]',
  meta_json     JSONB NOT NULL DEFAULT '{}',
  error_code    TEXT NOT NULL DEFAULT '',
  contract_id   UUID,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, contract_id) REFERENCES contracts(tenant_id, id),
  CHECK ((status = 'APPROVED') = (contract_id IS NOT NULL))
);

ALTER TABLE contract_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contract_drafts
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON contract_drafts TO matchwatch_app;
