package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

var ErrDuplicateEmail = errors.New("a user with this e-mail already exists")

type Credentials struct {
	UserID, TenantID uuid.UUID
	PasswordHash     string
}

// LookupLogin finds a user by e-mail without a tenant context (auth_lookup is SECURITY DEFINER; the app role has
// no privileges on users). ok=false when there is no such user.
func (s *Store) LookupLogin(ctx context.Context, email string) (Credentials, bool, error) {
	var c Credentials
	err := s.pool.QueryRow(ctx, `SELECT user_id, tenant_id, password_hash FROM auth_lookup($1)`, email).
		Scan(&c.UserID, &c.TenantID, &c.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, false, nil
	}
	return c, err == nil, wrap("auth lookup", err)
}

// UserEmail returns the e-mail of a user in the transaction's tenant.
func (tx *Tx) UserEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	var email string
	err := tx.QueryRow(ctx, `SELECT email FROM auth_user($1, $2)`, userID, tx.tenantID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return email, wrap("auth user", err)
}

func (tx *Tx) TenantName(ctx context.Context) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM tenants WHERE id=$1`, tx.tenantID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, wrap("tenant name", err)
}

// CreateUser needs the owner role (cmd/onboard); the app role cannot write users.
func (tx *Tx) CreateUser(ctx context.Context, email, passwordHash string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO users(tenant_id, email, password_hash) VALUES ($1,$2,$3) RETURNING id`,
		tx.tenantID, strings.ToLower(strings.TrimSpace(email)), passwordHash).Scan(&id)
	if isUniqueOn(err, "email") {
		return uuid.Nil, ErrDuplicateEmail
	}
	return id, wrap("create user", err)
}

// SignUp opens the transaction's tenant and its first user (auth_signup is SECURITY DEFINER). A taken e-mail is
// ErrDuplicateEmail; the transaction rolls back, so no empty tenant is left behind.
func (tx *Tx) SignUp(ctx context.Context, company, vkn, email, passwordHash string) (uuid.UUID, error) {
	if err := tx.InsertTenant(ctx, company, vkn, decimal.Zero); err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT user_id FROM auth_signup($1, $2, $3)`, tx.tenantID, email, passwordHash).Scan(&id)
	if isUniqueOn(err, "email") {
		return uuid.Nil, ErrDuplicateEmail
	}
	return id, wrap("sign up", err)
}

type UserRow struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type InviteRow struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Users lists the transaction tenant's logins, oldest first.
func (tx *Tx) Users(ctx context.Context) ([]UserRow, error) {
	rows, err := tx.Query(ctx, `SELECT user_id, email, created_at FROM auth_users($1)`, tx.tenantID)
	if err != nil {
		return nil, wrap("users", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[UserRow])
	return out, wrap("users", err)
}

// Invites lists the tenant's invites that are neither used nor expired, newest first.
func (tx *Tx) Invites(ctx context.Context) ([]InviteRow, error) {
	rows, err := tx.Query(ctx, `SELECT id, email, created_at, expires_at FROM auth_invites($1)`, tx.tenantID)
	if err != nil {
		return nil, wrap("invites", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[InviteRow])
	return out, wrap("invites", err)
}

// CreateInvite stores an invite link's hash for email; by is the inviting user (nil for a dev token). An e-mail
// that already has a login is ErrDuplicateEmail.
func (tx *Tx) CreateInvite(ctx context.Context, by *uuid.UUID, email string, hash []byte) (InviteRow, error) {
	inv := InviteRow{Email: strings.ToLower(strings.TrimSpace(email))}
	err := tx.QueryRow(ctx, `SELECT id, expires_at FROM auth_invite($1, $2, $3, $4)`, tx.tenantID, by, email, hash).
		Scan(&inv.ID, &inv.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return inv, ErrDuplicateEmail
	}
	inv.CreatedAt = time.Now()
	return inv, wrap("create invite", err)
}

// RevokeInvite deletes an unused invite of the tenant; anything else is ErrNotFound.
func (tx *Tx) RevokeInvite(ctx context.Context, id uuid.UUID) error {
	var n int64
	if err := tx.QueryRow(ctx, `SELECT auth_invite_revoke($1, $2)`, tx.tenantID, id).Scan(&n); err != nil {
		return wrap("revoke invite", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateResetLink stores a reset link's hash for a user of the tenant and returns that user's e-mail and the
// link's expiry; another tenant's user is ErrNotFound.
func (tx *Tx) CreateResetLink(ctx context.Context, userID uuid.UUID, hash []byte) (string, time.Time, error) {
	var email string
	var exp time.Time
	err := tx.QueryRow(ctx, `SELECT email, expires_at FROM auth_reset_issue($1, $2, $3)`, tx.tenantID, userID, hash).
		Scan(&email, &exp)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", exp, ErrNotFound
	}
	return email, exp, wrap("create reset link", err)
}

type LinkInfo struct {
	Kind     string // INVITE or RESET
	Email    string
	TenantID uuid.UUID
}

// LinkInfo tells what a still-valid link is for, without a tenant context; ok=false for an unknown, used or
// expired link.
func (s *Store) LinkInfo(ctx context.Context, hash []byte) (LinkInfo, bool, error) {
	var li LinkInfo
	err := s.pool.QueryRow(ctx, `SELECT kind, email, tenant_id FROM auth_token_info($1)`, hash).Scan(&li.Kind, &li.Email, &li.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return li, false, nil
	}
	return li, err == nil, wrap("link info", err)
}

// RedeemLink uses a link once: an invite creates the login, a reset sets the password. ok=false for an unknown,
// used or expired link; an invite whose e-mail got a login meanwhile is ErrDuplicateEmail and stays unused.
func (s *Store) RedeemLink(ctx context.Context, hash []byte, passwordHash string) (Credentials, bool, error) {
	var c Credentials
	err := s.pool.QueryRow(ctx, `SELECT user_id, tenant_id FROM auth_redeem($1, $2)`, hash, passwordHash).Scan(&c.UserID, &c.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, false, nil
	}
	if isUniqueOn(err, "email") {
		return c, false, ErrDuplicateEmail
	}
	return c, err == nil, wrap("redeem link", err)
}

// SessionOK reports whether a session token issued at iat for this user is still good: the user exists in the
// tenant and has not reset the password since.
func (s *Store) SessionOK(ctx context.Context, userID, tenantID uuid.UUID, iat time.Time) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT auth_session_ok($1, $2, $3)`, userID, tenantID, iat).Scan(&ok)
	return ok, wrap("session check", err)
}
