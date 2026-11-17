-- Faz 3a: kullanıcılar ve giriş. Giriş, tenant henüz bilinmeden kullanıcıyı e-postasından bulmak zorunda; bu yüzden bu
-- tabloda RLS yok. Onun yerine matchwatch_app'in bu tabloda hiçbir yetkisi yok ve tabloya yalnızca aşağıdaki iki
-- fonksiyonla ulaşır. Kullanıcıları owner rolü oluşturur (cmd/onboard), API asla oluşturmaz.
CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id),
  email         TEXT NOT NULL UNIQUE CHECK (email = lower(btrim(email)) AND email LIKE '%_@_%'),
  password_hash TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
REVOKE ALL ON users FROM PUBLIC;

-- En fazla bir satır döner; bcrypt hash'ini çağıran karşılaştırır.
CREATE FUNCTION auth_lookup(p_email TEXT) RETURNS TABLE (user_id UUID, tenant_id UUID, password_hash TEXT)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT u.id, u.tenant_id, u.password_hash FROM public.users u WHERE u.email = lower(btrim(p_email)) $$;

-- Yalnızca oturumun kendi tenant'ı: başka bir tenant'ın id'si satır döndürmez.
CREATE FUNCTION auth_user(p_user UUID, p_tenant UUID) RETURNS TABLE (email TEXT)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT u.email FROM public.users u WHERE u.id = p_user AND u.tenant_id = p_tenant $$;

REVOKE ALL ON FUNCTION auth_lookup(TEXT), auth_user(UUID, UUID) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_lookup(TEXT), auth_user(UUID, UUID) TO matchwatch_app;
