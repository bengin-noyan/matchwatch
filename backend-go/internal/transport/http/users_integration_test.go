package transporthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/audit"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// mailbox, API'nin gönderdiği e-postaları teslim etmek yerine kaydeder.
type mailbox struct {
	mu   sync.Mutex
	sent []struct{ to, subject, body string }
}

func (m *mailbox) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, struct{ to, subject, body string }{to, subject, body})
	return nil
}

// count, adrese kaç e-posta gittiğini söyler.
func (m *mailbox) count(to string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sent {
		if s.to == to {
			n++
		}
	}
	return n
}

// mailedToken, adrese giden n'inci e-postayı bekler (1'den başlayarak; sıfırlama e-postaları cevaptan sonra gider),
// içindeki bağlantının token'ını döner ve bağlantının sayfasını kontrol eder.
func (e *env) mailedToken(to, page string, n int) string {
	e.t.Helper()
	for i := 0; i < 100; i++ {
		e.mails.mu.Lock()
		var seen int
		for _, s := range e.mails.sent {
			if s.to != to {
				continue
			}
			if seen++; seen == n {
				e.mails.mu.Unlock()
				_, after, ok := strings.Cut(s.body, "https://mw.test"+page+"#")
				tok, _, _ := strings.Cut(after, "\n")
				if !ok || len(tok) != 43 {
					e.t.Fatalf("mail to %s has no %s link:\n%s", to, page, s.body)
				}
				return tok
			}
		}
		e.mails.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	e.t.Fatalf("no mail #%d to %s", n, to)
	return ""
}

func sessionOf(rec interface{ Result() *http.Response }) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}

// api, API'yi setup'ın dev token'ıyla çağırır (arkasında kullanıcı yok).
func (e *env) api(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.token)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

// loginAs, e-posta ve şifreyle giriş yapar ve oturum çerezini döner (reddedilirse nil).
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

	for _, bad := range []string{"yok-at", `a@b.c\r\nBcc: x@evil.com`, "a b@c.d"} {
		if rec := e.raw("POST", "/api/v1/invites", `{"email":"`+bad+`"}`, s); rec.Code != 422 {
			t.Errorf("bad e-mail %q: %d", bad, rec.Code)
		}
	}
	if rec := e.raw("POST", "/api/v1/invites", `{"email":"`+admin+`"}`, s); rec.Code != 409 || !strings.Contains(rec.Body.String(), "EMAIL_TAKEN") {
		t.Fatalf("invite an existing login: %d %s", rec.Code, rec.Body)
	}

	newbie := "Yeni-" + uuid.NewString()[:8] + "@Firma.com.tr"
	lower := strings.ToLower(newbie)
	invite := func() string {
		rec := e.raw("POST", "/api/v1/invites", `{"email":" `+newbie+` "}`, s)
		var out struct{ ID, Email, Token string }
		json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 201 || out.Email != lower || out.ID == "" || out.Token != "" || strings.Contains(rec.Body.String(), "token") {
			t.Fatalf("invite (the token must only go by mail): %d %s", rec.Code, rec.Body)
		}
		return out.ID
	}
	invite()
	id := invite() // ilkinin yerine geçer
	first, token := e.mailedToken(lower, "/davet", 1), e.mailedToken(lower, "/davet", 2)
	if e.mails.sent[len(e.mails.sent)-1].body == "" || !strings.Contains(e.mails.sent[len(e.mails.sent)-1].body, admin) {
		t.Fatalf("invite mail does not say who invited: %s", e.mails.sent[len(e.mails.sent)-1].body)
	}
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
	// yeni giriş, davet eden şirketin verilerini görür
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
	email := "iptal-" + uuid.NewString()[:8] + "@firma.com.tr"
	var out struct{ ID string }
	rec := a.api("POST", "/api/v1/invites", `{"email":"`+email+`"}`)
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 201 {
		t.Fatalf("invite: %d %s", rec.Code, rec.Body)
	}
	token := a.mailedToken(email, "/davet", 1)
	if rec := b.api("DELETE", "/api/v1/invites/"+out.ID, ""); rec.Code != 404 {
		t.Fatalf("other tenant revoked an invite: %d", rec.Code)
	}
	if rec := b.api("GET", "/api/v1/users", ""); strings.Contains(rec.Body.String(), out.ID) {
		t.Fatalf("other tenant sees the invite: %s", rec.Body)
	}
	if rec := a.api("DELETE", "/api/v1/invites/"+out.ID, ""); rec.Code != 204 {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if rec := a.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"yeni-uye-sifresi"}`, nil); rec.Code != 404 {
		t.Fatalf("revoked invite redeemed: %d", rec.Code)
	}
}

func TestForgotPasswordEndsOldSessions(t *testing.T) {
	e := setup(t, "9876543210")
	user := "unutkan-" + uuid.NewString()[:8] + "@firma.com.tr"
	e.addUser(user, "eski-sifre-123")
	old := e.loginAs(user, "eski-sifre-123")
	if old == nil {
		t.Fatal("login")
	}

	unknown := "yok-" + uuid.NewString()[:8] + "@firma.com.tr"
	for _, mail := range []string{unknown, " " + strings.ToUpper(user) + " "} {
		if rec := e.raw("POST", "/api/v1/auth/forgot", `{"email":"`+mail+`"}`, nil); rec.Code != 202 || rec.Body.Len() != 0 {
			t.Fatalf("forgot %s: %d %s", mail, rec.Code, rec.Body)
		}
	}
	token := e.mailedToken(user, "/sifre", 1)
	if e.mails.count(unknown) != 0 {
		t.Fatal("mailed an address without an account")
	}
	if rec := e.raw("POST", "/api/v1/auth/link", `{"token":"`+token+`"}`, nil); !strings.Contains(rec.Body.String(), `"kind":"RESET"`) {
		t.Fatalf("link info: %s", rec.Body)
	}

	time.Sleep(1100 * time.Millisecond) // JWT iat tam saniyedir; eski oturum daha önceki bir saniyeden olmalı
	rec := e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"yeni-sifre-4567"}`, nil)
	fresh := sessionOf(rec)
	if rec.Code != 200 || fresh == nil {
		t.Fatalf("redeem: %d %s", rec.Code, rec.Body)
	}
	if rec := e.raw("GET", "/api/v1/auth/me", "", old); rec.Code != 401 {
		t.Fatalf("session from before the reset still works: %d", rec.Code)
	}
	if rec := e.raw("GET", "/api/v1/auth/me", "", fresh); rec.Code != 200 {
		t.Fatalf("session the reset opened: %d", rec.Code)
	}
	if e.loginAs(user, "eski-sifre-123") != nil || e.loginAs(user, "yeni-sifre-4567") == nil {
		t.Fatal("password not replaced")
	}
	// dev token'ın arkasında kullanıcı yok, bundan etkilenmez
	if rec := e.api("GET", "/api/v1/users", ""); rec.Code != 200 {
		t.Fatalf("dev token: %d", rec.Code)
	}
}

func TestForgotPasswordLimit(t *testing.T) {
	e := setup(t, "9876543210")
	mail := "sel-" + uuid.NewString()[:8] + "@firma.com.tr"
	for i := 0; i < 5; i++ {
		e.raw("POST", "/api/v1/auth/forgot", `{"email":"`+mail+`"}`, nil)
	}
	if rec := e.raw("POST", "/api/v1/auth/forgot", `{"email":"`+mail+`"}`, nil); rec.Code != 429 {
		t.Fatalf("want 429 after five requests, got %d", rec.Code)
	}
}

func TestMailDisabled(t *testing.T) {
	e := setup(t, "9876543210")
	e.srv = NewRouter(e.store, audit.New(e.store, nil, nil), e.storage, secret, nil, 0, false, nil, "")
	if rec := e.api("POST", "/api/v1/invites", `{"email":"a@b.c"}`); rec.Code != 503 || !strings.Contains(rec.Body.String(), "MAIL_DISABLED") {
		t.Fatalf("invite without mail: %d %s", rec.Code, rec.Body)
	}
	if rec := e.raw("POST", "/api/v1/auth/forgot", `{"email":"a@b.c"}`, nil); rec.Code != 503 {
		t.Fatalf("forgot without mail: %d", rec.Code)
	}
}

func TestExpiredLink(t *testing.T) {
	e := setup(t, "9876543210")
	email := "gec-" + uuid.NewString()[:8] + "@firma.com.tr"
	var out struct{ ID string }
	json.Unmarshal(e.api("POST", "/api/v1/invites", `{"email":"`+email+`"}`).Body.Bytes(), &out)
	token := e.mailedToken(email, "/davet", 1)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	if _, err := owner.Exec(ctx, `UPDATE auth_tokens SET expires_at = now() - interval '1 second' WHERE id = $1`, out.ID); err != nil {
		t.Fatal(err)
	}
	if rec := e.raw("POST", "/api/v1/auth/link", `{"token":"`+token+`"}`, nil); rec.Code != 404 {
		t.Fatalf("expired link info: %d", rec.Code)
	}
	if rec := e.raw("POST", "/api/v1/auth/redeem", `{"token":"`+token+`","password":"yeni-uye-sifresi"}`, nil); rec.Code != 404 {
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
