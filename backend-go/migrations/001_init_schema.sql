-- matchwatch şeması (spec bölüm 4, D1–D15). Embedding sütunları Faz 2'ye bırakıldı (D8).

CREATE TABLE tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  vkn VARCHAR(11) NOT NULL,
  plan_tier TEXT NOT NULL DEFAULT 'free',
  financing_rate_annual NUMERIC(7,4) NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vendors (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  vkn VARCHAR(11) NOT NULL,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, vkn),
  UNIQUE (tenant_id, id)
);

CREATE TABLE contracts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  vendor_id UUID NOT NULL,
  contract_no TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','INACTIVE')),
  valid_from DATE NOT NULL,
  valid_until DATE NOT NULL,
  payment_term_days INT NOT NULL DEFAULT 0 CHECK (payment_term_days >= 0),
  currency VARCHAR(3) NOT NULL,
  qty_tolerance_pct NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (qty_tolerance_pct >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, contract_no),
  CHECK (valid_from <= valid_until),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, vendor_id) REFERENCES vendors(tenant_id, id)
);

CREATE TABLE contract_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  contract_id UUID NOT NULL,
  item_code TEXT NOT NULL DEFAULT '',
  barcode TEXT NOT NULL DEFAULT '',
  item_name TEXT NOT NULL,
  unit_price NUMERIC(19,6) NOT NULL CHECK (unit_price >= 0),
  currency VARCHAR(3) NOT NULL,
  uom TEXT NOT NULL,
  withholding_code VARCHAR(10),
  requires_delivery BOOLEAN NOT NULL DEFAULT true,
  source_clause_text TEXT NOT NULL DEFAULT '',
  source_page INT NOT NULL DEFAULT 0,
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, contract_id) REFERENCES contracts(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX contract_items_contract_idx ON contract_items (tenant_id, contract_id);

CREATE TABLE despatch_advices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  vendor_id UUID NOT NULL,
  ettn UUID NOT NULL,
  despatch_no TEXT NOT NULL,
  issue_date DATE NOT NULL,
  shipment_date DATE NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, ettn),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, vendor_id) REFERENCES vendors(tenant_id, id)
);
CREATE INDEX despatch_advices_vendor_idx ON despatch_advices (tenant_id, vendor_id, issue_date);

CREATE TABLE despatch_lines (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  despatch_id UUID NOT NULL,
  line_id TEXT NOT NULL,
  item_code TEXT NOT NULL DEFAULT '',
  barcode TEXT NOT NULL DEFAULT '',
  item_name TEXT NOT NULL DEFAULT '',
  uom TEXT NOT NULL DEFAULT '',
  delivered_qty NUMERIC(19,4) NOT NULL CHECK (delivered_qty >= 0),
  rejected_qty NUMERIC(19,4) NOT NULL DEFAULT 0 CHECK (rejected_qty >= 0),
  CHECK (rejected_qty <= delivered_qty),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, despatch_id) REFERENCES despatch_advices(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX despatch_lines_item_idx ON despatch_lines (tenant_id, item_code);

CREATE TABLE invoices (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  vendor_id UUID NOT NULL,
  ettn UUID NOT NULL,
  invoice_no TEXT NOT NULL,
  issue_date DATE NOT NULL,
  due_date DATE,
  currency VARCHAR(3) NOT NULL,
  fx_source VARCHAR(3),
  fx_target VARCHAR(3),
  fx_rate NUMERIC(19,6),
  payable_minor BIGINT NOT NULL,
  audit_status TEXT NOT NULL DEFAULT 'PENDING' CHECK (audit_status IN ('PENDING','GREEN','YELLOW','RED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, ettn),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, vendor_id) REFERENCES vendors(tenant_id, id)
);
CREATE INDEX invoices_vendor_idx ON invoices (tenant_id, vendor_id, issue_date);

CREATE TABLE invoice_lines (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  invoice_id UUID NOT NULL,
  line_id TEXT NOT NULL,
  item_code TEXT NOT NULL DEFAULT '',
  barcode TEXT NOT NULL DEFAULT '',
  item_name TEXT NOT NULL DEFAULT '',
  uom TEXT NOT NULL DEFAULT '',
  qty NUMERIC(19,4) NOT NULL,
  unit_price NUMERIC(19,6) NOT NULL,
  allowance_amount_minor BIGINT NOT NULL DEFAULT 0,
  line_extension_minor BIGINT NOT NULL,
  vat_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
  withholding_code VARCHAR(10),
  withholding_rate NUMERIC(5,2),
  withholding_amount_minor BIGINT NOT NULL DEFAULT 0,
  matched_contract_item_id UUID,
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, invoice_id) REFERENCES invoices(tenant_id, id) ON DELETE CASCADE,
  -- MATCH SIMPLE: matched_contract_item_id NULL ise kontrol atlanır
  FOREIGN KEY (tenant_id, matched_contract_item_id) REFERENCES contract_items(tenant_id, id)
);
CREATE INDEX invoice_lines_invoice_idx ON invoice_lines (tenant_id, invoice_id);
CREATE INDEX invoice_lines_item_idx ON invoice_lines (tenant_id, item_code);

CREATE TABLE invoice_despatch_refs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  invoice_id UUID NOT NULL,
  despatch_no TEXT NOT NULL,
  issue_date DATE,
  FOREIGN KEY (tenant_id, invoice_id) REFERENCES invoices(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX invoice_despatch_refs_invoice_idx ON invoice_despatch_refs (tenant_id, invoice_id);

CREATE TABLE despatch_allocations (
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  invoice_line_id UUID NOT NULL,
  despatch_line_id UUID NOT NULL,
  qty NUMERIC(19,4) NOT NULL CHECK (qty > 0),
  PRIMARY KEY (invoice_line_id, despatch_line_id),
  FOREIGN KEY (tenant_id, invoice_line_id) REFERENCES invoice_lines(tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, despatch_line_id) REFERENCES despatch_lines(tenant_id, id)
);
CREATE INDEX despatch_allocations_despatch_idx ON despatch_allocations (tenant_id, despatch_line_id);

CREATE TABLE audit_findings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  invoice_id UUID NOT NULL,
  invoice_line_id UUID,
  severity TEXT NOT NULL CHECK (severity IN ('CRITICAL','WARNING','INFO')),
  rule_code TEXT NOT NULL CHECK (rule_code IN ('PRICE_MISMATCH','QTY_EXCEEDS_DELIVERY','UNCONTRACTED_ITEM',
    'MATURITY_VIOLATION','DUPLICATE_SUSPECT','WITHHOLDING_ERROR','PRICE_CREEP','FX_UNVERIFIED','UOM_MISMATCH')),
  expected_value TEXT NOT NULL DEFAULT '',
  actual_value TEXT NOT NULL DEFAULT '',
  financial_impact_minor BIGINT NOT NULL DEFAULT 0,
  impact_currency VARCHAR(3),
  evidence_json JSONB NOT NULL,
  resolution_status TEXT NOT NULL DEFAULT 'OPEN' CHECK (resolution_status IN ('OPEN','DISPUTED','APPROVED_EXCEPTION')),
  resolution_note TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, invoice_id) REFERENCES invoices(tenant_id, id) ON DELETE CASCADE,
  -- MATCH SIMPLE: invoice_line_id NULL ise kontrol atlanır
  FOREIGN KEY (tenant_id, invoice_line_id) REFERENCES invoice_lines(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX audit_findings_invoice_idx ON audit_findings (tenant_id, invoice_id);

-- D9: zorunlu RLS. NULLIF, transaction'a özel bir set_config'ten sonra havuzdaki oturumlarda kalan '' değerini
-- karşılar.
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenants
  USING (id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE vendors ENABLE ROW LEVEL SECURITY;
ALTER TABLE vendors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON vendors
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE contracts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contracts
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE contract_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contract_items
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE despatch_advices ENABLE ROW LEVEL SECURITY;
ALTER TABLE despatch_advices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON despatch_advices
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE despatch_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE despatch_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON despatch_lines
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invoices
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE invoice_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invoice_lines
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE invoice_despatch_refs ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoice_despatch_refs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invoice_despatch_refs
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE despatch_allocations ENABLE ROW LEVEL SECURITY;
ALTER TABLE despatch_allocations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON despatch_allocations
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

ALTER TABLE audit_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_findings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_findings
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, vendors, contracts, contract_items, despatch_advices,
  despatch_lines, invoices, invoice_lines, invoice_despatch_refs, despatch_allocations, audit_findings
  TO matchwatch_app;
