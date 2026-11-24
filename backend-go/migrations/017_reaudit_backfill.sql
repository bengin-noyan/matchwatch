-- 016'dan önce yarım kalan yeniden denetimlerin izi yok; o yüzden var olan faturaların hepsi bir kez yeniden denetlenir
-- (D11: idempotent, insan kararları korunur). FORCE RLS owner'ı da bağladığı için tenant tenant gidilir.
DO $$
DECLARE t UUID;
BEGIN
  FOR t IN SELECT DISTINCT tenant_id FROM users LOOP
    PERFORM set_config('app.current_tenant_id', t::text, true);
    UPDATE invoices SET reaudit_since = now() WHERE tenant_id = t AND reaudit_since IS NULL;
  END LOOP;
  PERFORM set_config('app.current_tenant_id', '', true);
END $$;
