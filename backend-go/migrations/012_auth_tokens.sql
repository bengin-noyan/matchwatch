-- Faz 5a: invites and password resets by one-time link. Like users, auth_tokens has no RLS (a link is redeemed
-- before any tenant is known); matchwatch_app gets no privileges on either table and reaches them only through the
-- functions below. Only the SHA-256 of a token is stored: the token itself is shown once, to the person who made it.
CREATE TABLE auth_tokens (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash BYTEA NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  kind       TEXT NOT NULL CHECK (kind IN ('INVITE', 'RESET')),
  tenant_id  UUID NOT NULL REFERENCES tenants(id),
  email      TEXT NOT NULL CHECK (email = lower(btrim(email)) AND email LIKE '%_@_%'),
  created_by UUID, -- users.id without an FK, like finding_decisions; NULL = operator (cmd/resetlink) or a dev token
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  used_at    TIMESTAMPTZ
);
REVOKE ALL ON auth_tokens FROM PUBLIC;

-- The tenant's logins, oldest first.
CREATE FUNCTION auth_users(p_tenant UUID) RETURNS TABLE (user_id UUID, email TEXT, created_at TIMESTAMPTZ)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT u.id, u.email, u.created_at FROM public.users u WHERE u.tenant_id = p_tenant ORDER BY u.created_at, u.id $$;

-- Invites not yet used and not expired, newest first.
CREATE FUNCTION auth_invites(p_tenant UUID) RETURNS TABLE (id UUID, email TEXT, created_at TIMESTAMPTZ, expires_at TIMESTAMPTZ)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT t.id, t.email, t.created_at, t.expires_at FROM public.auth_tokens t
   WHERE t.tenant_id = p_tenant AND t.kind = 'INVITE' AND t.used_at IS NULL AND t.expires_at > now()
   ORDER BY t.created_at DESC, t.id $$;

-- A new invite replaces the tenant's earlier unused ones for that e-mail. No row when the e-mail already has a login
-- (in any tenant: e-mails are globally unique).
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

-- A reset link for a user of the tenant; it replaces that user's earlier unused ones. No row for another tenant's user.
CREATE FUNCTION auth_reset_issue(p_tenant UUID, p_user UUID, p_by UUID, p_hash BYTEA) RETURNS TABLE (email TEXT, expires_at TIMESTAMPTZ)
  LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ WITH u AS (SELECT u.email FROM public.users u WHERE u.id = p_user AND u.tenant_id = p_tenant),
   gone AS (DELETE FROM public.auth_tokens t USING u WHERE t.kind = 'RESET' AND t.email = u.email AND t.used_at IS NULL)
   INSERT INTO public.auth_tokens(token_hash, kind, tenant_id, email, created_by, expires_at)
   SELECT p_hash, 'RESET', p_tenant, u.email, p_by, now() + interval '1 hour' FROM u
   RETURNING auth_tokens.email, auth_tokens.expires_at $$;

-- What a still-valid link is for; no row for an unknown, used or expired one.
CREATE FUNCTION auth_token_info(p_hash BYTEA) RETURNS TABLE (kind TEXT, email TEXT, tenant_id UUID)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, public AS
$$ SELECT t.kind, t.email, t.tenant_id FROM public.auth_tokens t
   WHERE t.token_hash = p_hash AND t.used_at IS NULL AND t.expires_at > now() $$;

-- Uses a link once: an invite creates the login, a reset sets the new password. The UPDATE takes the row lock, so of
-- two racing redeems only one gets a row. An invite whose e-mail got a login meanwhile raises a unique violation and
-- the whole call, including used_at, rolls back.
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
    RETURN QUERY UPDATE public.users u SET password_hash = p_password_hash
      WHERE u.email = t.email AND u.tenant_id = t.tenant_id RETURNING u.id, u.tenant_id;
  END IF;
END
$$;

REVOKE ALL ON FUNCTION auth_users(UUID), auth_invites(UUID), auth_invite(UUID, UUID, TEXT, BYTEA),
  auth_invite_revoke(UUID, UUID), auth_reset_issue(UUID, UUID, UUID, BYTEA), auth_token_info(BYTEA),
  auth_redeem(BYTEA, TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_users(UUID), auth_invites(UUID), auth_invite(UUID, UUID, TEXT, BYTEA),
  auth_invite_revoke(UUID, UUID), auth_reset_issue(UUID, UUID, UUID, BYTEA), auth_token_info(BYTEA),
  auth_redeem(BYTEA, TEXT) TO matchwatch_app;
