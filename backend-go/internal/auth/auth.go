// Package auth, tenant id'sini taşıyan HS256 JWT'leri üretir ve doğrular (spec D13).
package auth

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type claims struct {
	TenantID string `json:"tenant_id"`
	jwt.RegisteredClaims
}

type ctxKey struct{}

type subKey struct{}

// CookieName, web uygulamasının oturum JWT'sini taşıyan çerezin adı (HttpOnly, SameSite=Strict). Betikler header
// kullanmaya devam eder.
const CookieName = "mw_session"

// Sign, tenantID için ttl sonra süresi dolan bir HS256 token üretir.
func Sign(secret []byte, tenantID uuid.UUID, sub string, ttl time.Duration) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		TenantID: tenantID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}).SignedString(secret)
}

// TenantID, Middleware'in kaydettiği tenant'ı döner (Middleware dışında uuid.Nil).
func TenantID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxKey{}).(uuid.UUID)
	return id
}

// Subject, Middleware'in kaydettiği token'ın sub değerini döner: web girişlerinde kullanıcı id'si, dev token'larda
// "devtoken".
func Subject(ctx context.Context) string {
	s, _ := ctx.Value(subKey{}).(string)
	return s
}

// SessionCheck, bir kullanıcının iat anında açılmış oturumu hâlâ geçerli mi diye bakar: kullanıcı duruyor mu ve o
// zamandan beri şifresini sıfırlamış mı.
type SessionCheck func(ctx context.Context, userID, tenantID uuid.UUID, iat time.Time) (bool, error)

// Middleware geçerli bir token ister: önce Bearer header'ına, yoksa oturum çerezine bakar. Yalnızca HS256 kabul edilir,
// exp zorunludur, tenant_id bir UUID olmalıdır. sub bir kullanıcı id'siyse token check'ten de geçmelidir; dev
// token'ların (sub "devtoken") arkasında kullanıcı yoktur.
func Middleware(secret []byte, check SessionCheck) func(http.Handler) http.Handler {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				if c, err := r.Cookie(CookieName); err == nil {
					raw, ok = c.Value, true
				}
			}
			if !ok || raw == "" {
				unauthorized(w)
				return
			}
			var c claims
			if _, err := parser.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return secret, nil }); err != nil {
				unauthorized(w)
				return
			}
			tid, err := uuid.Parse(c.TenantID)
			if err != nil || tid == uuid.Nil {
				unauthorized(w)
				return
			}
			if uid, err := uuid.Parse(c.Subject); err == nil {
				if c.IssuedAt == nil {
					unauthorized(w)
					return
				}
				ok, err := check(r.Context(), uid, tid, c.IssuedAt.Time)
				if err != nil {
					log.Printf("session check: %v", err)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error":"internal error","code":"INTERNAL"}`))
					return
				}
				if !ok {
					unauthorized(w)
					return
				}
			}
			ctx := context.WithValue(context.WithValue(r.Context(), ctxKey{}, tid), subKey{}, c.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"missing or invalid token","code":"UNAUTHORIZED"}`))
}
