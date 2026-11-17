-- Faz 3k: kendi kendine kayıt. API tenant'ı kendisi açar (RLS altında tenants'a INSERT yapabilir) ve o tenant'ın ilk
-- kullanıcısını bu fonksiyonla ekler; uygulama rolünün users tablosunda hâlâ hiçbir yetkisi yok. Zaten kullanıcısı olan
-- bir tenant için satır dönmez; yani bu fonksiyon var olan bir şirkete giriş ekleyemez.
CREATE FUNCTION auth_signup(p_tenant UUID, p_email TEXT, p_hash TEXT) RETURNS TABLE (user_id UUID)
  LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ INSERT INTO public.users(tenant_id, email, password_hash)
   SELECT p_tenant, lower(btrim(p_email)), p_hash
   WHERE NOT EXISTS (SELECT 1 FROM public.users u WHERE u.tenant_id = p_tenant)
   RETURNING id $$;

REVOKE ALL ON FUNCTION auth_signup(UUID, TEXT, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_signup(UUID, TEXT, TEXT) TO matchwatch_app;
