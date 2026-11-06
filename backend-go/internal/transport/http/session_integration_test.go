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

// addUser creates a login for e's tenant with the owner role, as cmd/onboard does, and returns its id.
func (e *env) addUser(email, password string) uuid.UUID {
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
	var id uuid.UUID
	if err := repository.NewStore(owner).InTenant(ctx, e.tid, func(tx *repository.Tx) (err error) {
		id, err = tx.CreateUser(ctx, email, string(hash))
		return err
	}); err != nil {
		e.t.Fatal(err)
	}
	return id
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

func TestSignup(t *testing.T) {
	e := setup(t, "9876543210")
	email := "Kayit-" + uuid.NewString()[:8] + "@Yeni.com.tr"
	body := func(vkn, mail, pw string) string {
		return `{"company":"Yeni Firma A.Ş.","vkn":"` + vkn + `","email":"` + mail + `","password":"` + pw + `"}`
	}
	for name, c := range map[string]struct {
		body, code string
	}{
		"bad vkn":        {body("9876543210", email, "uzun-bir-sifre-1"), "INVALID_VKN"},
		"short password": {body("1234567890", email, "kisa"), "WEAK_PASSWORD"},
		"bad email":      {body("1234567890", "yok-at-isareti", "uzun-bir-sifre-1"), "INVALID_EMAIL"},
		"no company":     {`{"company":" ","vkn":"1234567890","email":"a@b.c","password":"uzun-bir-sifre-1"}`, "INVALID_COMPANY"},
	} {
		if rec := e.raw("POST", "/api/v1/auth/signup", c.body, nil); rec.Code != 422 || !strings.Contains(rec.Body.String(), c.code) {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}

	rec := e.raw("POST", "/api/v1/auth/signup", body("1234567890", email, "uzun-bir-sifre-1"), nil)
	if rec.Code != 201 {
		t.Fatalf("signup: %d %s", rec.Code, rec.Body)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatal("signup set no session cookie")
	}
	lower := strings.ToLower(email)
	if rec := e.raw("GET", "/api/v1/auth/me", "", session); rec.Code != 200 || !strings.Contains(rec.Body.String(), lower) || !strings.Contains(rec.Body.String(), "Yeni Firma A.Ş.") {
		t.Fatalf("me after signup: %d %s", rec.Code, rec.Body)
	}
	// a new, empty company: none of the setup tenant's data
	if rec := e.raw("GET", "/api/v1/invoices", "", session); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Fatalf("new tenant invoices: %d %s", rec.Code, rec.Body)
	}
	if rec := e.raw("POST", "/api/v1/auth/login", `{"email":"`+lower+`","password":"uzun-bir-sifre-1"}`, nil); rec.Code != 200 {
		t.Fatalf("login after signup: %d", rec.Code)
	}

	if rec := e.raw("POST", "/api/v1/auth/signup", body("1234567808", " "+lower+" ", "baska-bir-sifre-2"), nil); rec.Code != 409 || !strings.Contains(rec.Body.String(), "EMAIL_TAKEN") {
		t.Fatalf("duplicate e-mail: %d %s", rec.Code, rec.Body)
	}
	// two attempts so far from this address; the limit is five an hour
	for i := 0; i < 3; i++ {
		e.raw("POST", "/api/v1/auth/signup", body("1234567808", lower, "baska-bir-sifre-2"), nil)
	}
	if rec := e.raw("POST", "/api/v1/auth/signup", body("1234567808", "yeni-"+lower, "baska-bir-sifre-2"), nil); rec.Code != 429 {
		t.Fatalf("want 429 after five sign-ups, got %d", rec.Code)
	}
}

func TestSignupFunctionOnlyAddsAFirstUser(t *testing.T) {
	e := setup(t, "9876543210")
	e.addUser("ilk-"+uuid.NewString()[:8]+"@firma.com.tr", "dogru-sifre-123")
	second := "ikinci-" + uuid.NewString()[:8] + "@firma.com.tr"
	var n int
	err := e.store.InTenant(context.Background(), e.tid, func(tx *repository.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM auth_signup($1, $2, 'x')`, e.tid, second).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.store.LookupLogin(context.Background(), second); n != 0 || ok {
		t.Fatalf("auth_signup added a second user to an existing company (rows %d)", n)
	}
}
