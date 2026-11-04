-- Faz 3a: users and login. Login must find a user by e-mail before any tenant is known, so this table has no RLS;
-- instead matchwatch_app gets NO privileges on it and reaches it only through the two functions below.
-- Users are created by the owner role (cmd/onboard), never by the API.
CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id),
  email         TEXT NOT NULL UNIQUE CHECK (email = lower(btrim(email)) AND email LIKE '%_@_%'),
  password_hash TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
REVOKE ALL ON users FROM PUBLIC;

-- Returns at most one row; the caller compares the bcrypt hash.
CREATE FUNCTION auth_lookup(p_email TEXT) RETURNS TABLE (user_id UUID, tenant_id UUID, password_hash TEXT)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT u.id, u.tenant_id, u.password_hash FROM public.users u WHERE u.email = lower(btrim(p_email)) $$;

-- Only the session's own tenant: an id from another tenant yields no row.
CREATE FUNCTION auth_user(p_user UUID, p_tenant UUID) RETURNS TABLE (email TEXT)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT u.email FROM public.users u WHERE u.id = p_user AND u.tenant_id = p_tenant $$;

REVOKE ALL ON FUNCTION auth_lookup(TEXT), auth_user(UUID, UUID) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_lookup(TEXT), auth_user(UUID, UUID) TO matchwatch_app;
