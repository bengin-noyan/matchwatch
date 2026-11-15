package transporthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

func sessionOf(rec interface{ Result() *http.Response }) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}

// api calls the API with the setup's dev token (no user behind it).
func (e *env) api(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.token)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

// loginAs signs in with e-mail and password and returns the session cookie (nil when refused).
func (e *env) loginAs(email, pw string) *http.Cookie {
	return sessionOf(e.raw("POST", "/api/v1/auth/login", `{"email":"`+email+`","password":"`+pw+`"}`, nil))
}

func TestInviteFlow(t *testing.T) {
	e := setup(t, "9876543210")
	admin := "yonetici-" + uuid.NewString()[:8] + "@firma.com.tr"
	e.addUser(admin, "dogru-sifre-123")
	s := e.loginAs(admin, "dogru-sifre-123")
	if s == nil {
		t.Fatal("login")
	}

	if rec := e.raw("POST", "/api/v1/invites", `{"email":"yok-at"}`, s); rec.Code != 422 {
		t.Fatalf("bad e-mail: %d", rec.Code)
	}
	if rec := e.raw("POST", "/api/v1/invites", `{"email":"`+admin+`"}`, s); rec.Code != 409 || !strings.Contains(rec.Body.String(), "EMAIL_TAKEN") {
		t.Fatalf("invite an existing login: %d %s", rec.Code, rec.Body)
	}

	newbie := "Yeni-" + uuid.NewString()[:8] + "@Firma.com.tr"
	lower := strings.ToLower(newbie)
	invite := func() (id, token string) {
		rec := e.raw("POST", "/api/v1/invites", `{"email":" `+newbie+` "}`, s)
		var out struct{ ID, Email, Token string }
		json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 201 || out.Email != lower || len(out.Token) != 43 {
			t.Fatalf("invite: %d %s", rec.Code, rec.Body)
		}
		return out.ID, out.Token
	}
	_, first := invite()
	id, token := invite() // replaces the first
	if rec := e.raw("POST", "/api/v1/auth/link", `{"token":"`+first+`"}`, nil); rec.Code != 404 {
		t.Fatalf("replaced invite still valid: %d", rec.Code)
	}

	rec := e.raw("GET", "/api/v1/users", "", s)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), admin) || strings.Count(rec.Body.String(), lower) != 1 || strings.Contains(rec.Body.String(), token) {
		t.Fatalf("users: %d %s", rec.Code, rec.Body)
	}

	rec = e.raw("POST", "/api/v1/auth/link", `{"token":"`+token+`"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"kind":"INVITE"`) || !strings.Contains(rec.Body.String(), `"tenant_name":"api-`) {
		t.Fatalf("link info: %d %s", rec.Code, rec.Body)
	}
	if rec := e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"kisa"}`, nil); rec.Code != 422 {
		t.Fatalf("weak password: %d", rec.Code)
	}
	rec = e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"yeni-uye-sifresi"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), lower) || sessionOf(rec) == nil {
		t.Fatalf("redeem: %d %s", rec.Code, rec.Body)
	}
	// the new login sees the inviting company's data
	if rec := e.raw("GET", "/api/v1/users", "", sessionOf(rec)); !strings.Contains(rec.Body.String(), admin) {
		t.Fatalf("new user in another tenant: %s", rec.Body)
	}
	if rec := e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"baska-bir-sifre"}`, nil); rec.Code != 404 {
		t.Fatalf("second redeem: %d", rec.Code)
	}
	if e.loginAs(lower, "yeni-uye-sifresi") == nil {
		t.Fatal("login with the invited account")
	}
	if rec := e.raw("DELETE", "/api/v1/invites/"+id, "", s); rec.Code != 404 {
		t.Fatalf("revoke a used invite: %d", rec.Code)
	}
}

func TestInviteRevokeAndTenantIsolation(t *testing.T) {
	a := setup(t, "9876543210")
	b := setup(t, "9876543210")
	invite := func(e *env) (id, token string) {
		var out struct{ ID, Token string }
		rec := e.api("POST", "/api/v1/invites", `{"email":"iptal-`+uuid.NewString()[:8]+`@firma.com.tr"}`)
		json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 201 {
			t.Fatalf("invite: %d %s", rec.Code, rec.Body)
		}
		return out.ID, out.Token
	}
	id, token := invite(a)
	if rec := b.api("DELETE", "/api/v1/invites/"+id, ""); rec.Code != 404 {
		t.Fatalf("other tenant revoked an invite: %d", rec.Code)
	}
	if rec := b.api("GET", "/api/v1/users", ""); strings.Contains(rec.Body.String(), id) {
		t.Fatalf("other tenant sees the invite: %s", rec.Body)
	}
	if rec := a.api("DELETE", "/api/v1/invites/"+id, ""); rec.Code != 204 {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if rec := a.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"yeni-uye-sifresi"}`, nil); rec.Code != 404 {
		t.Fatalf("revoked invite redeemed: %d", rec.Code)
	}
}

func TestResetLink(t *testing.T) {
	e := setup(t, "9876543210")
	other := setup(t, "9876543210")
	user := "unutkan-" + uuid.NewString()[:8] + "@firma.com.tr"
	uid := e.addUser(user, "eski-sifre-123")

	if rec := other.api("POST", "/api/v1/users/"+uid.String()+"/reset-link", ""); rec.Code != 404 {
		t.Fatalf("other tenant issued a reset link: %d", rec.Code)
	}
	issue := func() string {
		var out struct{ Email, Token string }
		rec := e.api("POST", "/api/v1/users/"+uid.String()+"/reset-link", "")
		json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 201 || out.Email != user {
			t.Fatalf("reset link: %d %s", rec.Code, rec.Body)
		}
		return out.Token
	}
	first := issue()
	token := issue()
	if rec := e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+first+`","password":"yeni-sifre-4567"}`, nil); rec.Code != 404 {
		t.Fatalf("replaced reset link still works: %d", rec.Code)
	}
	if rec := e.raw("POST", "/api/v1/auth/link", `{"token":"`+token+`"}`, nil); !strings.Contains(rec.Body.String(), `"kind":"RESET"`) {
		t.Fatalf("link info: %s", rec.Body)
	}
	rec := e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"yeni-sifre-4567"}`, nil)
	if rec.Code != 200 || sessionOf(rec) == nil {
		t.Fatalf("redeem: %d %s", rec.Code, rec.Body)
	}
	if e.loginAs(user, "eski-sifre-123") != nil || e.loginAs(user, "yeni-sifre-4567") == nil {
		t.Fatal("password not replaced")
	}
}

func TestExpiredLink(t *testing.T) {
	e := setup(t, "9876543210")
	var out struct{ ID, Token string }
	rec := e.api("POST", "/api/v1/invites", `{"email":"gec-`+uuid.NewString()[:8]+`@firma.com.tr"}`)
	json.Unmarshal(rec.Body.Bytes(), &out)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	if _, err := owner.Exec(ctx, `UPDATE auth_tokens SET expires_at = now() - interval '1 second' WHERE id = $1`, out.ID); err != nil {
		t.Fatal(err)
	}
	if rec := e.raw("POST", "/api/v1/auth/link", `{"token":"`+out.Token+`"}`, nil); rec.Code != 404 {
		t.Fatalf("expired link info: %d", rec.Code)
	}
	if rec := e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+out.Token+`","password":"yeni-uye-sifresi"}`, nil); rec.Code != 404 {
		t.Fatalf("expired link redeemed: %d", rec.Code)
	}
}

func TestAppRoleCannotReadAuthTokens(t *testing.T) {
	e := setup(t, "9876543210")
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		_, err := tx.Exec(context.Background(), `SELECT 1 FROM auth_tokens`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("app role read auth_tokens: %v", err)
	}
}
