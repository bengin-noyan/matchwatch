package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var secret = []byte(strings.Repeat("s", 32))

// call, middleware'i verilen Authorization header'ıyla çalıştırır; HTTP durumunu ve handler'ın gördüğü tenant'ı döner.
func call(t *testing.T, header string) (int, uuid.UUID) {
	t.Helper()
	var seen uuid.UUID
	h := Middleware(secret, revoked)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = TenantID(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, seen
}

// revokedUser'ın oturumları kapatıldı (şifresini sıfırladı); diğer herkesinki geçerli.
var revokedUser = uuid.New()

func revoked(_ context.Context, uid, _ uuid.UUID, _ time.Time) (bool, error) {
	return uid != revokedUser, nil
}

func TestSessionCheck(t *testing.T) {
	tid := uuid.New()
	live, _ := Sign(secret, tid, uuid.NewString(), time.Hour)
	ended, _ := Sign(secret, tid, revokedUser.String(), time.Hour)
	if code, _ := call(t, "Bearer "+live); code != 200 {
		t.Errorf("live user: %d", code)
	}
	if code, _ := call(t, "Bearer "+ended); code != 401 {
		t.Errorf("ended session: %d", code)
	}
}

func TestValidToken(t *testing.T) {
	tid := uuid.New()
	tok, err := Sign(secret, tid, "tester", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if code, seen := call(t, "Bearer "+tok); code != 200 || seen != tid {
		t.Fatalf("code %d tenant %s want %s", code, seen, tid)
	}
}

func TestRejected(t *testing.T) {
	tid := uuid.New()
	good, _ := Sign(secret, tid, "x", time.Hour)
	expired, _ := Sign(secret, tid, "x", -time.Minute)
	wrongSecret, _ := Sign([]byte(strings.Repeat("o", 32)), tid, "x", time.Hour)
	claims := func(m jwt.MapClaims, method jwt.SigningMethod, key any) string {
		s, err := jwt.NewWithClaims(method, m).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	exp := time.Now().Add(time.Hour).Unix()
	cases := map[string]string{
		"missing":      "",
		"not bearer":   "Basic " + good,
		"expired":      "Bearer " + expired,
		"wrong secret": "Bearer " + wrongSecret,
		"alg none":     "Bearer " + claims(jwt.MapClaims{"tenant_id": tid.String(), "exp": exp}, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType),
		"hs384":        "Bearer " + claims(jwt.MapClaims{"tenant_id": tid.String(), "exp": exp}, jwt.SigningMethodHS384, secret),
		"no exp":       "Bearer " + claims(jwt.MapClaims{"tenant_id": tid.String()}, jwt.SigningMethodHS256, secret),
		"bad tenant":   "Bearer " + claims(jwt.MapClaims{"tenant_id": "nope", "exp": exp}, jwt.SigningMethodHS256, secret),
		"no tenant":    "Bearer " + claims(jwt.MapClaims{"exp": exp}, jwt.SigningMethodHS256, secret),
	}
	for name, h := range cases {
		if code, _ := call(t, h); code != 401 {
			t.Errorf("%s: code %d, want 401", name, code)
		}
	}
}
