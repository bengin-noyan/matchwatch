-- Faz 3k: self sign-up. The API opens a tenant itself (it may INSERT tenants under RLS) and adds that tenant's first
-- user through this function; the app role still has no privileges on users. A tenant that already has a user
-- gets no row back, so this cannot add a login to an existing company.
CREATE FUNCTION auth_signup(p_tenant UUID, p_email TEXT, p_hash TEXT) RETURNS TABLE (user_id UUID)
  LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ INSERT INTO public.users(tenant_id, email, password_hash)
   SELECT p_tenant, lower(btrim(p_email)), p_hash
   WHERE NOT EXISTS (SELECT 1 FROM public.users u WHERE u.tenant_id = p_tenant)
   RETURNING id $$;

REVOKE ALL ON FUNCTION auth_signup(UUID, TEXT, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_signup(UUID, TEXT, TEXT) TO matchwatch_app;
