package transporthttp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/auth"
	"github.com/bengin-noyan/matchwatch/backend-go/internal/repository"
)

// Invites and password resets are one-time links mailed to the address they are for. Nobody else ever sees the
// token, so no colleague (and no inviter) can use a link meant for someone else. Every user of a company may invite;
// there are no roles yet.

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

const mailTimeout = 20 * time.Second

func (s *server) mailDisabled(w http.ResponseWriter) bool {
	if s.mailer == nil {
		writeErr(w, 503, "MAIL_DISABLED", "e-mail is not configured on this server (MAIL_URL)")
		return true
	}
	return false
}

func (s *server) postInvite(w http.ResponseWriter, r *http.Request) {
	if s.mailDisabled(w) {
		return
	}
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
	var tenant, inviter string
	err := s.store.InTenant(r.Context(), auth.TenantID(r.Context()), func(tx *repository.Tx) (err error) {
		if inv, err = tx.CreateInvite(r.Context(), actor(r), email, hash); err != nil {
			return err
		}
		if by := actor(r); by != nil {
			if inviter, err = tx.UserEmail(r.Context(), *by); err != nil {
				return err
			}
		}
		tenant, err = tx.TenantName(r.Context())
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
	who := tenant
	if inviter != "" {
		who = inviter + " (" + tenant + ")"
	}
	body := "Merhaba,\n\n" + who + " sizi matchwatch'a davet etti. Şifrenizi belirleyip hesabınızı açmak için:\n\n" +
		s.baseURL + "/davet#" + token + "\n\nBağlantı 7 gün geçerlidir ve bir kez kullanılabilir. Bu daveti beklemiyorsanız " +
		"bu e-postayı yok sayabilirsiniz.\n\nmatchwatch\n"
	ctx, cancel := context.WithTimeout(r.Context(), mailTimeout)
	defer cancel()
	if err := s.mailer.Send(ctx, inv.Email, "matchwatch daveti: "+tenant, body); err != nil {
		// the invite stays but nobody has its token; inviting again replaces it
		log.Printf("invite mail: %v", err)
		writeErr(w, 502, "MAIL_FAILED", "the invite e-mail could not be sent; try again")
		return
	}
	writeJSON(w, 201, inv)
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

// forgot ("Şifremi unuttum", no session) mails a reset link when the e-mail has a login. It always answers 202 at
// once and does the work after the response, so neither the answer nor its timing tells whether an account exists.
func (s *server) forgot(w http.ResponseWriter, r *http.Request) {
	if s.mailDisabled(w) {
		return
	}
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
	ip, _, err := net.SplitHostPort(r.RemoteAddr) // the client, behind Caddy (see signup)
	if err != nil {
		ip = r.RemoteAddr
	}
	// per address and per mailbox, so nobody can flood one inbox from many addresses
	if s.forgots.Blocked("ip:"+ip) || s.forgots.Blocked("mail:"+email) {
		writeErr(w, 429, "TOO_MANY_RESETS", "too many reset requests; try again later")
		return
	}
	s.forgots.Fail("ip:" + ip)
	s.forgots.Fail("mail:" + email)
	go s.mailReset(email)
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) mailReset(email string) {
	ctx, cancel := context.WithTimeout(context.Background(), mailTimeout)
	defer cancel()
	cred, ok, err := s.store.LookupLogin(ctx, email)
	if err != nil || !ok {
		if err != nil {
			log.Printf("reset lookup: %v", err)
		}
		return
	}
	token, hash := auth.NewLinkToken()
	if err := s.store.InTenant(ctx, cred.TenantID, func(tx *repository.Tx) error {
		_, _, err := tx.CreateResetLink(ctx, cred.UserID, hash)
		return err
	}); err != nil {
		log.Printf("reset link: %v", err)
		return
	}
	body := "Merhaba,\n\nmatchwatch hesabınız için şifre sıfırlama istendi. Yeni şifrenizi belirlemek için:\n\n" +
		s.baseURL + "/sifre#" + token + "\n\nBağlantı 1 saat geçerlidir ve bir kez kullanılabilir. Yeni şifreyi kaydettiğinizde " +
		"açık bütün oturumlarınız kapanır. Bunu siz istemediyseniz bu e-postayı yok sayın; şifreniz değişmez.\n\nmatchwatch\n"
	if err := s.mailer.Send(ctx, email, "matchwatch şifre sıfırlama", body); err != nil {
		log.Printf("reset mail: %v", err)
	}
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
