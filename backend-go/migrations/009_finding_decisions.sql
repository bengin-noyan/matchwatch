-- Faz 3f: every human decision on a finding (who, when, which note), append-only.
-- Keyed by the finding's identity (invoice, line, rule), not its id: a re-audit deletes and recreates OPEN
-- findings, so a reopened finding gets a new id, and its trail must survive that.
-- No backfill: rows written before this migration have no known decider; the finding still shows its current note.
CREATE TABLE finding_decisions (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID NOT NULL REFERENCES tenants(id),
  invoice_id        UUID NOT NULL,
  invoice_line_id   UUID,
  rule_code         TEXT NOT NULL,
  resolution_status TEXT NOT NULL CHECK (resolution_status IN ('OPEN','DISPUTED','APPROVED_EXCEPTION')),
  note              TEXT,
  -- users.id without an FK: the trail must outlive the user. The e-mail is a snapshot taken at decision time;
  -- both are NULL for dev tokens.
  decided_by        UUID,
  decided_by_email  TEXT,
  decided_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, invoice_id) REFERENCES invoices(tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, invoice_line_id) REFERENCES invoice_lines(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX finding_decisions_invoice_idx ON finding_decisions (tenant_id, invoice_id, decided_at);

ALTER TABLE finding_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE finding_decisions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON finding_decisions
  USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
  WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- Append-only for the app: no UPDATE or DELETE.
GRANT SELECT, INSERT ON finding_decisions TO matchwatch_app;
