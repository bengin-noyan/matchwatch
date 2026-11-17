-- Faz 3f: bir bulgu üzerine verilen her insan kararı (kim, ne zaman, hangi notla); yalnızca eklenir, değiştirilmez.
-- Satırlar bulgunun id'sine değil kimliğine (fatura, satır, kural) bağlı: yeniden denetim OPEN bulguları silip yeniden
-- oluşturur, yani yeniden açılan bir bulgu yeni bir id alır ve geçmişi bunu atlatmalı.
-- Geriye dönük doldurma yok: bu migration'dan önceki kararların kim tarafından verildiği bilinmiyor; bulgu yine de
-- güncel notunu gösterir.
CREATE TABLE finding_decisions (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID NOT NULL REFERENCES tenants(id),
  invoice_id        UUID NOT NULL,
  invoice_line_id   UUID,
  rule_code         TEXT NOT NULL,
  resolution_status TEXT NOT NULL CHECK (resolution_status IN ('OPEN','DISPUTED','APPROVED_EXCEPTION')),
  note              TEXT,
  -- users.id için FK yok: geçmiş, kullanıcı silinse de kalmalı. E-posta karar anında alınmış bir kopyadır; dev
  -- token'larda ikisi de NULL.
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

-- Uygulama için yalnızca ekleme: UPDATE ya da DELETE yok.
GRANT SELECT, INSERT ON finding_decisions TO matchwatch_app;
