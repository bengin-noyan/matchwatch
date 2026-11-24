-- Yarım kalan denetimlerin yeniden denenmesi bütün tenant'ları gezer, ama RLS altında uygulama rolü tenants'ı
-- listeleyemez (FORCE yüzünden owner da). users'ta RLS olmadığı için tenant'lar oradan gelir; kullanıcısı olmayan bir
-- tenant'a zaten kimse giremez. Yalnızca id döner; faturalar yine tenant transaction'ında okunur.
CREATE FUNCTION audit_tenants() RETURNS TABLE (tenant_id UUID)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT DISTINCT u.tenant_id FROM public.users u $$;

REVOKE ALL ON FUNCTION audit_tenants() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION audit_tenants() TO matchwatch_app;
