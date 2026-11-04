package transporthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// addUser creates a login for e's tenant with the owner role, as cmd/onboard does.
func (e *env) addUser(email, password string) {
	e.t.Helper()
	ctx := context.Background()
	owner, err := pgxpool.New(ctx, os.Getenv("MATCHWATCH_TEST_MIGRATE_URL"))
	if err != nil {
		e.t.Fatal(err)
	}
	defer owner.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := repository.NewStore(owner).InTenant(ctx, e.tid, func(tx *repository.Tx) error {
		_, err := tx.CreateUser(ctx, email, string(hash))
		return err
	}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) raw(method, path string, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func TestLoginCookieSession(t *testing.T) {
	e := setup(t, "9876543210")
	email := "Denetci-" + uuid.NewString()[:8] + "@Firma.com.tr"
	e.addUser(email, "dogru-sifre-123")
	lower := strings.ToLower(email)

	for name, body := range map[string]string{
		"wrong password": `{"email":"` + email + `","password":"yanlis"}`,
		"unknown email":  `{"email":"yok-` + uuid.NewString()[:8] + `@firma.com.tr","password":"dogru-sifre-123"}`,
	} {
		rec := e.raw("POST", "/api/v1/auth/login", body, nil)
		if rec.Code != 401 || !strings.Contains(rec.Body.String(), "INVALID_LOGIN") || rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	if rec := e.raw("POST", "/api/v1/auth/login", `nope`, nil); rec.Code != 400 {
		t.Errorf("bad body: %d", rec.Code)
	}

	rec := e.raw("POST", "/api/v1/auth/login", `{"email":" `+email+` ","password":"dogru-sifre-123"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	var me struct {
		Email      string `json:"email"`
		TenantName string `json:"tenant_name"`
	}
	json.Unmarshal(rec.Body.Bytes(), &me)
	if me.Email != lower || !strings.HasPrefix(me.TenantName, "api-") {
		t.Fatalf("me %+v", me)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || session.Path != "/api" || session.MaxAge != 8*3600 {
		t.Fatalf("cookie %+v", session)
	}
	if rec := e.raw("GET", "/api/v1/invoices", "", session); rec.Code != 200 {
		t.Fatalf("cookie session rejected: %d", rec.Code)
	}
	if rec := e.raw("GET", "/api/v1/auth/me", "", session); rec.Code != 200 || !strings.Contains(rec.Body.String(), lower) {
		t.Fatalf("me: %d %s", rec.Code, rec.Body)
	}
	rec = e.raw("POST", "/api/v1/auth/logout", "", session)
	if rec.Code != 204 || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout: %d %v", rec.Code, rec.Header())
	}
	if rec := e.raw("GET", "/api/v1/auth/me", "", nil); rec.Code != 401 {
		t.Fatalf("me without session: %d", rec.Code)
	}
}

func TestLoginLockout(t *testing.T) {
	e := setup(t, "9876543210")
	email := "kilit-" + uuid.NewString()[:8] + "@firma.com.tr"
	e.addUser(email, "dogru-sifre-123")
	for i := 0; i < 10; i++ {
		e.raw("POST", "/api/v1/auth/login", `{"email":"`+email+`","password":"x"}`, nil)
	}
	// even the right password is refused while locked out
	if rec := e.raw("POST", "/api/v1/auth/login", `{"email":"`+email+`","password":"dogru-sifre-123"}`, nil); rec.Code != 429 {
		t.Fatalf("want 429, got %d", rec.Code)
	}
}

func TestAppRoleCannotReadUsers(t *testing.T) {
	e := setup(t, "9876543210")
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		_, err := tx.Exec(context.Background(), `SELECT 1 FROM users`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("app role read users: %v", err)
	}
}
