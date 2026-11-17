-- Faz 5a: tek kullanımlık bağlantıyla davet ve şifre sıfırlama. Bağlantı yalnızca ilgili adrese e-postayla gider,
-- isteyen kişiye asla gösterilmez; böylece kimse bir iş arkadaşının hesabını ele geçiremez. users gibi auth_tokens'ta
-- da RLS yok (bağlantı, tenant bilinmeden kullanılır); matchwatch_app'in iki tabloda da hiçbir yetkisi yok ve onlara
-- yalnızca aşağıdaki fonksiyonlarla ulaşır. Token'ın yalnızca SHA-256'sı saklanır.

-- Şifre sıfırlama, ondan önce açılmış bütün oturumları kapatır (middleware her istekte auth_session_ok'a sorar).
ALTER TABLE users ADD COLUMN sessions_valid_after TIMESTAMPTZ NOT NULL DEFAULT '-infinity';
CREATE TABLE auth_tokens (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash BYTEA NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  kind       TEXT NOT NULL CHECK (kind IN ('INVITE', 'RESET')),
  tenant_id  UUID NOT NULL REFERENCES tenants(id),
  email      TEXT NOT NULL CHECK (email = lower(btrim(email)) AND email LIKE '%_@_%'),
  created_by UUID, -- daveti gönderen users.id, FK'siz (finding_decisions gibi); sıfırlamalarda ve dev token'larda NULL
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  used_at    TIMESTAMPTZ
);
REVOKE ALL ON auth_tokens FROM PUBLIC;

-- Tenant'ın girişleri, en eskiden başlayarak.
CREATE FUNCTION auth_users(p_tenant UUID) RETURNS TABLE (user_id UUID, email TEXT, created_at TIMESTAMPTZ)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT u.id, u.email, u.created_at FROM public.users u WHERE u.tenant_id = p_tenant ORDER BY u.created_at, u.id $$;

-- Henüz kullanılmamış ve süresi dolmamış davetler, en yeniden başlayarak.
CREATE FUNCTION auth_invites(p_tenant UUID) RETURNS TABLE (id UUID, email TEXT, created_at TIMESTAMPTZ, expires_at TIMESTAMPTZ)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT t.id, t.email, t.created_at, t.expires_at FROM public.auth_tokens t
   WHERE t.tenant_id = p_tenant AND t.kind = 'INVITE' AND t.used_at IS NULL AND t.expires_at > now()
   ORDER BY t.created_at DESC, t.id $$;

-- Yeni bir davet, tenant'ın o e-posta için önceki kullanılmamış davetlerinin yerine geçer. E-postanın zaten bir girişi
-- varsa satır dönmez (hangi tenant'ta olursa olsun: e-postalar her yerde benzersiz).
CREATE FUNCTION auth_invite(p_tenant UUID, p_by UUID, p_email TEXT, p_hash BYTEA) RETURNS TABLE (id UUID, expires_at TIMESTAMPTZ)
  LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ WITH gone AS (DELETE FROM public.auth_tokens t
                 WHERE t.tenant_id = p_tenant AND t.kind = 'INVITE' AND t.email = lower(btrim(p_email)) AND t.used_at IS NULL)
   INSERT INTO public.auth_tokens(token_hash, kind, tenant_id, email, created_by, expires_at)
   SELECT p_hash, 'INVITE', p_tenant, lower(btrim(p_email)), p_by, now() + interval '7 days'
   WHERE NOT EXISTS (SELECT 1 FROM public.users u WHERE u.email = lower(btrim(p_email)))
   RETURNING auth_tokens.id, auth_tokens.expires_at $$;

CREATE FUNCTION auth_invite_revoke(p_tenant UUID, p_id UUID) RETURNS BIGINT
  LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ WITH d AS (DELETE FROM public.auth_tokens t
              WHERE t.id = p_id AND t.tenant_id = p_tenant AND t.kind = 'INVITE' AND t.used_at IS NULL RETURNING 1)
   SELECT count(*) FROM d $$;

-- Tenant'taki bir kullanıcı için sıfırlama bağlantısı ("Şifremi unuttum" ya da cmd/resetlink); o kullanıcının önceki
-- kullanılmamış bağlantılarının yerine geçer. Başka bir tenant'ın kullanıcısı için satır dönmez.
CREATE FUNCTION auth_reset_issue(p_tenant UUID, p_user UUID, p_hash BYTEA) RETURNS TABLE (email TEXT, expires_at TIMESTAMPTZ)
  LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ WITH u AS (SELECT u.email FROM public.users u WHERE u.id = p_user AND u.tenant_id = p_tenant),
   gone AS (DELETE FROM public.auth_tokens t USING u WHERE t.kind = 'RESET' AND t.email = u.email AND t.used_at IS NULL)
   INSERT INTO public.auth_tokens(token_hash, kind, tenant_id, email, created_by, expires_at)
   SELECT p_hash, 'RESET', p_tenant, u.email, NULL, now() + interval '1 hour' FROM u
   RETURNING auth_tokens.email, auth_tokens.expires_at $$;

-- Hâlâ geçerli bir bağlantının ne için olduğu; bilinmeyen, kullanılmış ya da süresi dolmuş bağlantı için satır dönmez.
CREATE FUNCTION auth_token_info(p_hash BYTEA) RETURNS TABLE (kind TEXT, email TEXT, tenant_id UUID)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT t.kind, t.email, t.tenant_id FROM public.auth_tokens t
   WHERE t.token_hash = p_hash AND t.used_at IS NULL AND t.expires_at > now() $$;

-- Bir oturum token'ı (sub = kullanıcı, p_iat anında verilmiş) hâlâ geçerli mi? Kullanıcı silindiyse ya da token'dan
-- sonra şifresini sıfırladıysa değil. JWT iat tam saniye olduğu için sessions_valid_after da saniyeye yuvarlanır:
-- sıfırlamanın kendi açtığı oturum geçerli kalır (aynı saniyede, ondan hemen önce açılmış bir oturum da geçerli
-- kalırdı).
CREATE FUNCTION auth_session_ok(p_user UUID, p_tenant UUID, p_iat TIMESTAMPTZ) RETURNS BOOLEAN
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT EXISTS (SELECT 1 FROM public.users u WHERE u.id = p_user AND u.tenant_id = p_tenant AND u.sessions_valid_after <= p_iat) $$;

-- Bir bağlantıyı tek seferlik kullanır: davet girişi oluşturur, sıfırlama yeni şifreyi koyar ve eski oturumları
-- kapatır. UPDATE satırı kilitler, yani aynı anda gelen iki kullanımdan yalnızca biri satır alır. Bu arada e-postası
-- için giriş açılmış bir davet benzersizlik hatası verir ve used_at dahil bütün çağrı geri alınır.
CREATE FUNCTION auth_redeem(p_hash BYTEA, p_password_hash TEXT) RETURNS TABLE (user_id UUID, tenant_id UUID)
  LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$
#variable_conflict use_column
DECLARE t public.auth_tokens%ROWTYPE;
BEGIN
  UPDATE public.auth_tokens a SET used_at = now()
   WHERE a.token_hash = p_hash AND a.used_at IS NULL AND a.expires_at > now()
   RETURNING a.* INTO t;
  IF NOT FOUND THEN
    RETURN;
  END IF;
  IF t.kind = 'INVITE' THEN
    RETURN QUERY INSERT INTO public.users AS u (tenant_id, email, password_hash)
      VALUES (t.tenant_id, t.email, p_password_hash) RETURNING u.id, u.tenant_id;
  ELSE
    RETURN QUERY UPDATE public.users u SET password_hash = p_password_hash, sessions_valid_after = date_trunc('second', now())
      WHERE u.email = t.email AND u.tenant_id = t.tenant_id RETURNING u.id, u.tenant_id;
  END IF;
END
$$;

REVOKE ALL ON FUNCTION auth_users(UUID), auth_invites(UUID), auth_invite(UUID, UUID, TEXT, BYTEA),
  auth_invite_revoke(UUID, UUID), auth_reset_issue(UUID, UUID, BYTEA), auth_token_info(BYTEA),
  auth_session_ok(UUID, UUID, TIMESTAMPTZ), auth_redeem(BYTEA, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_users(UUID), auth_invites(UUID), auth_invite(UUID, UUID, TEXT, BYTEA),
  auth_invite_revoke(UUID, UUID), auth_reset_issue(UUID, UUID, BYTEA), auth_token_info(BYTEA),
  auth_session_ok(UUID, UUID, TIMESTAMPTZ), auth_redeem(BYTEA, TEXT) TO matchwatch_app;
