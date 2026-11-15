package transporthttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// Invites and password resets travel as one-time links that a person hands over: there is no mail delivery yet
// (its provider is a Faz 4 decision, KVKK). Every user of a company may invite and issue reset links; there are no
// roles yet.
// shortcut: a reset does not end sessions already open (JWTs are stateless, 8 h); revoke them when roles or a
// session table arrive.

// actor is the signed-in user, nil for a dev token (sub is not a user id).
func actor(r *http.Request) *uuid.UUID {
	if id, err := uuid.Parse(auth.Subject(r.Context())); err == nil {
		return &id
	}
	return nil
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Users   []repository.UserRow   `json:"users"`
		Invites []repository.InviteRow `json:"invites"`
	}{[]repository.UserRow{}, []repository.InviteRow{}}
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		if out.Users, err = tx.Users(r.Context()); err != nil {
			return err
		}
		out.Invites, err = tx.Invites(r.Context())
		return err
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, out)
}

type linkOut struct {
	ID        *uuid.UUID `json:"id,omitempty"`
	Email     string     `json:"email"`
	Token     string     `json:"token"` // shown once; only its hash is stored
	ExpiresAt time.Time  `json:"expires_at"`
}

func (s *server) postInvite(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be a JSON object with email")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(email) {
		writeErr(w, 422, "INVALID_EMAIL", "email is not a valid address")
		return
	}
	token, hash := auth.NewLinkToken()
	var inv repository.InviteRow
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		inv, err = tx.CreateInvite(r.Context(), actor(r), email, hash)
		return err
	})
	if errors.Is(err, repository.ErrDuplicateEmail) {
		writeErr(w, 409, "EMAIL_TAKEN", "an account with this e-mail already exists")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, linkOut{&inv.ID, inv.Email, token, inv.ExpiresAt})
}

func (s *server) deleteInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	if err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) error {
		return tx.RevokeInvite(r.Context(), id)
	}); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) postResetLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(r)
	if !ok {
		writeErr(w, 404, "NOT_FOUND", "not found")
		return
	}
	token, hash := auth.NewLinkToken()
	out := linkOut{Token: token}
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		out.Email, out.ExpiresAt, err = tx.CreateResetLink(r.Context(), userID, actor(r), hash)
		return err
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, out)
}

// linkInfo (no session) tells the link page what a link is for. The token comes in the body, never the URL, so it
// stays out of access logs; the web app keeps it in the URL fragment.
func (s *server) linkInfo(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be a JSON object with token")
		return
	}
	li, ok, err := s.store.LinkInfo(r.Context(), auth.LinkTokenHash(req.Token))
	if err != nil {
		fail(w, err)
		return
	}
	if !ok {
		writeErr(w, 404, "INVALID_LINK", "this link is unknown, used or expired")
		return
	}
	var tenant string
	if err := s.store.InTenant(r.Context(), li.TenantID, func(tx *repository.Tx) (err error) {
		tenant, err = tx.TenantName(r.Context())
		return err
	}); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"kind": li.Kind, "email": li.Email, "tenant_name": tenant})
}

// redeemLink (no session) sets the password from an invite or reset link and logs the user in.
func (s *server) redeemLink(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "body must be a JSON object with token and password")
		return
	}
	if !validPassword(req.Password) {
		writeErr(w, 422, "WEAK_PASSWORD", "password must be 12 characters to 72 bytes")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), auth.PasswordCost)
	if err != nil {
		fail(w, err)
		return
	}
	cred, ok, err := s.store.RedeemLink(r.Context(), auth.LinkTokenHash(req.Token), string(hash))
	switch {
	case errors.Is(err, repository.ErrDuplicateEmail):
		writeErr(w, 409, "EMAIL_TAKEN", "an account with this e-mail already exists")
		return
	case err != nil:
		fail(w, err)
		return
	case !ok:
		writeErr(w, 404, "INVALID_LINK", "this link is unknown, used or expired")
		return
	}
	tok, err := auth.Sign(s.secret, cred.TenantID, cred.UserID.String(), sessionTTL)
	if err != nil {
		fail(w, err)
		return
	}
	http.SetCookie(w, s.sessionCookie(tok, int(sessionTTL.Seconds())))
	s.writeMe(w, r, cred.TenantID, cred.UserID.String())
}
