// Package auth issues and verifies the HS256 JWTs that carry the tenant id (spec D13).
package auth

import (
	"context"
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

// CookieName carries the session JWT for the web app (HttpOnly, SameSite=Strict); scripts keep using the header.
const CookieName = "mw_session"

// Sign returns an HS256 token for tenantID that expires after ttl.
func Sign(secret []byte, tenantID uuid.UUID, sub string, ttl time.Duration) (string, error) {
	now := time.Now()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		TenantID: tenantID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}).SignedString(secret)
}

// TenantID returns the tenant stored by Middleware (uuid.Nil outside it).
func TenantID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxKey{}).(uuid.UUID)
	return id
}

// Subject returns the token's sub claim stored by Middleware (a user id for web logins, "devtoken" for dev tokens).
func Subject(ctx context.Context) string {
	s, _ := ctx.Value(subKey{}).(string)
	return s
}

// Middleware requires a valid token, from the Bearer header or else the session cookie:
// HS256 only, exp required, tenant_id a UUID.
func Middleware(secret []byte) func(http.Handler) http.Handler {
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
