// Package repository, Postgres ile konuşan tek koddur. Her tenant sorgusu Store.InTenant içinde çalışır.
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var (
	ErrDuplicateETTN     = errors.New("repository: duplicate ETTN")
	ErrCustomerMismatch  = errors.New("repository: customer VKN does not match tenant")
	ErrNotFound          = errors.New("repository: not found")
	ErrUnknownDespatch   = errors.New("repository: unknown despatch")
	ErrDuplicateContract = errors.New("repository: duplicate contract number")
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Tx, app.current_tenant_id'si önceden ayarlanmış bir transaction'dır.
type Tx struct {
	pgx.Tx
	tenantID uuid.UUID
}

// InTenant, fn'i tenantID'ye bağlı bir transaction'da çalıştırır; hata ya da panic olursa geri alır (panic'i sonra
// yeniden fırlatır).
func (s *Store) InTenant(ctx context.Context, tenantID uuid.UUID, fn func(tx *Tx) error) (err error) {
	ptx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			ptx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			ptx.Rollback(ctx)
		}
	}()
	if _, err = ptx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", tenantID.String()); err != nil {
		return err
	}
	if err = fn(&Tx{Tx: ptx, tenantID: tenantID}); err != nil {
		return err
	}
	return ptx.Commit(ctx)
}

// CreateTenant id'yi Go'da üretir, böylece kayıttan önce RLS ayarı yapılabilir.
func (s *Store) CreateTenant(ctx context.Context, name, vkn string, financingRate decimal.Decimal) (uuid.UUID, error) {
	id := uuid.New()
	err := s.InTenant(ctx, id, func(tx *Tx) error { return tx.InsertTenant(ctx, name, vkn, financingRate) })
	return id, err
}

// InsertTenant, transaction'ın kendi tenant'ını oluşturur (id'si InTenant'a verilen id'dir).
func (tx *Tx) InsertTenant(ctx context.Context, name, vkn string, financingRate decimal.Decimal) error {
	_, err := tx.Exec(ctx, `INSERT INTO tenants(id, name, vkn, financing_rate_annual) VALUES ($1,$2,$3,$4)`,
		tx.tenantID, name, vkn, num(financingRate))
	return err
}

// num, bir ondalığı float'a hiç uğramadan pgx'in numeric tipine çevirir.
func num(d decimal.Decimal) pgtype.Numeric {
	return pgtype.Numeric{Int: d.Coefficient(), Exp: d.Exponent(), Valid: true}
}

// isUniqueOn, adında part geçen bir kısıtta 23505 (benzersizlik) ihlali olup olmadığını söyler.
func isUniqueOn(err error, part string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, part)
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}

// Ping, veritabanına bağlanılabiliyor mu diye bakar (/healthz kullanır).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
