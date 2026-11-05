package transporthttp

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

const sessionTTL = 8 * time.Hour

// dummyHash keeps a login for an unknown e-mail as slow as a wrong password, so timing does not reveal accounts.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("matchwatch-no-such-user"), auth.PasswordCost)

func (s *server) sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: auth.CookieName, Value: value, Path: "/api", MaxAge: maxAge,
		HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteStrictMode}
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be a JSON object with email and password")
		return
	}
	key := strings.ToLower(strings.TrimSpace(req.Email))
	if s.limiter.Blocked(key) {
		writeErr(w, 429, "TOO_MANY_ATTEMPTS", "too many failed logins for this e-mail; try again in 15 minutes")
		return
	}
	cred, found, err := s.store.LookupLogin(r.Context(), key)
	if err != nil {
		fail(w, err)
		return
	}
	hash := dummyHash
	if found {
		hash = []byte(cred.PasswordHash)
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(req.Password)) != nil || !found {
		s.limiter.Fail(key)
		writeErr(w, 401, "INVALID_LOGIN", "e-mail or password is wrong")
		return
	}
	s.limiter.Reset(key)
	tok, err := auth.Sign(s.secret, cred.TenantID, cred.UserID.String(), sessionTTL)
	if err != nil {
		fail(w, err)
		return
	}
	http.SetCookie(w, s.sessionCookie(tok, int(sessionTTL.Seconds())))
	s.writeMe(w, r, cred.TenantID, cred.UserID.String())
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.sessionCookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	s.writeMe(w, r, auth.TenantID(r.Context()), auth.Subject(r.Context()))
}

// writeMe answers {email, tenant_name}; a dev token (sub is not a user id) has no e-mail.
func (s *server) writeMe(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, sub string) {
	var out struct {
		Email      string `json:"email"`
		TenantName string `json:"tenant_name"`
	}
	err := s.store.InTenant(r.Context(), tenantID, func(tx *repository.Tx) error {
		var err error
		if out.TenantName, err = tx.TenantName(r.Context()); err != nil {
			return err
		}
		if uid, perr := uuid.Parse(sub); perr == nil {
			out.Email, err = tx.UserEmail(r.Context(), uid)
		}
		return err
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, out)
}
