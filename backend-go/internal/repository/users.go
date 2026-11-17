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

// LookupLogin, bir kullanıcıyı tenant bilmeden e-postasından bulur (auth_lookup SECURITY DEFINER; uygulama rolünün
// users tablosunda hiçbir yetkisi yok). Böyle bir kullanıcı yoksa ok=false.
func (s *Store) LookupLogin(ctx context.Context, email string) (Credentials, bool, error) {
	var c Credentials
	err := s.pool.QueryRow(ctx, `SELECT user_id, tenant_id, password_hash FROM auth_lookup($1)`, email).
		Scan(&c.UserID, &c.TenantID, &c.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, false, nil
	}
	return c, err == nil, wrap("auth lookup", err)
}

// UserEmail, transaction'ın tenant'ındaki bir kullanıcının e-postasını döner.
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

// CreateUser owner rolü ister (cmd/onboard); uygulama rolü users tablosuna yazamaz.
func (tx *Tx) CreateUser(ctx context.Context, email, passwordHash string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO users(tenant_id, email, password_hash) VALUES ($1,$2,$3) RETURNING id`,
		tx.tenantID, strings.ToLower(strings.TrimSpace(email)), passwordHash).Scan(&id)
	if isUniqueOn(err, "email") {
		return uuid.Nil, ErrDuplicateEmail
	}
	return id, wrap("create user", err)
}

// SignUp, transaction'ın tenant'ını ve ilk kullanıcısını açar (auth_signup SECURITY DEFINER). E-posta alınmışsa
// ErrDuplicateEmail döner; transaction geri alınır, yani arkada boş bir tenant kalmaz.
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

// Users, transaction'ın tenant'ındaki girişleri en eskiden başlayarak listeler.
func (tx *Tx) Users(ctx context.Context) ([]UserRow, error) {
	rows, err := tx.Query(ctx, `SELECT user_id, email, created_at FROM auth_users($1)`, tx.tenantID)
	if err != nil {
		return nil, wrap("users", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[UserRow])
	return out, wrap("users", err)
}

// Invites, tenant'ın ne kullanılmış ne de süresi dolmuş davetlerini en yeniden başlayarak listeler.
func (tx *Tx) Invites(ctx context.Context) ([]InviteRow, error) {
	rows, err := tx.Query(ctx, `SELECT id, email, created_at, expires_at FROM auth_invites($1)`, tx.tenantID)
	if err != nil {
		return nil, wrap("invites", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[InviteRow])
	return out, wrap("invites", err)
}

// CreateInvite, email için bir davet bağlantısının hash'ini saklar; by daveti gönderen kullanıcıdır (dev token'da nil).
// Zaten girişi olan bir e-posta ErrDuplicateEmail döner.
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

// RevokeInvite, tenant'ın kullanılmamış bir davetini siler; başka her durumda ErrNotFound döner.
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

// CreateResetLink, tenant'taki bir kullanıcı için sıfırlama bağlantısının hash'ini saklar; o kullanıcının e-postasını
// ve bağlantının bitiş zamanını döner. Başka bir tenant'ın kullanıcısı için ErrNotFound döner.
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
	Kind     string // INVITE ya da RESET
	Email    string
	TenantID uuid.UUID
}

// LinkInfo, hâlâ geçerli bir bağlantının ne için olduğunu tenant bilmeden söyler. Bilinmeyen, kullanılmış ya da süresi
// dolmuş bağlantı için ok=false.
func (s *Store) LinkInfo(ctx context.Context, hash []byte) (LinkInfo, bool, error) {
	var li LinkInfo
	err := s.pool.QueryRow(ctx, `SELECT kind, email, tenant_id FROM auth_token_info($1)`, hash).Scan(&li.Kind, &li.Email, &li.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return li, false, nil
	}
	return li, err == nil, wrap("link info", err)
}

// RedeemLink bir bağlantıyı tek seferlik kullanır: davet girişi oluşturur, sıfırlama şifreyi değiştirir. Bilinmeyen,
// kullanılmış ya da süresi dolmuş bağlantı için ok=false. Bu arada e-postası için giriş açılmış bir davet
// ErrDuplicateEmail döner ve kullanılmamış olarak kalır.
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

// SessionOK, bu kullanıcı için iat anında verilmiş oturum token'ının hâlâ geçerli olup olmadığını söyler: kullanıcı
// tenant'ta duruyor ve o zamandan beri şifresini sıfırlamamış olmalı.
func (s *Store) SessionOK(ctx context.Context, userID, tenantID uuid.UUID, iat time.Time) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT auth_session_ok($1, $2, $3)`, userID, tenantID, iat).Scan(&ok)
	return ok, wrap("session check", err)
}
