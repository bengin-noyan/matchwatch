package repository

import (
	"context"
	"errors"
	"strings"

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
