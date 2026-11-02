// Package repository implements PostgreSQL persistence. Every query on
// tenant-owned data takes the tenant ID explicitly and filters on it, so a
// caller can never read or modify another tenant's rows by guessing IDs.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a row does not exist (within the tenant).
var ErrNotFound = errors.New("not found")

// DBTX is satisfied by both *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store groups all repositories over one DBTX (pool or transaction).
type Store struct {
	pool *pgxpool.Pool
	db   DBTX

	Tenants     *TenantRepo
	Users       *UserRepo
	Roles       *RoleRepo
	Permissions *PermissionRepo
	Tokens      *TokenRepo
	Inventory   *InventoryRepo
}

func NewStore(pool *pgxpool.Pool) *Store { return newStore(pool, pool) }

func newStore(pool *pgxpool.Pool, db DBTX) *Store {
	return &Store{
		pool:        pool,
		db:          db,
		Tenants:     &TenantRepo{db: db},
		Users:       &UserRepo{db: db},
		Roles:       &RoleRepo{db: db},
		Permissions: &PermissionRepo{db: db},
		Tokens:      &TokenRepo{db: db},
		Inventory:   &InventoryRepo{db: db},
	}
}

// WithTx runs fn inside a transaction; it commits on nil error and rolls back
// otherwise (including panics).
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) (err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(newStore(s.pool, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// IsUniqueViolation reports whether err violates the named unique constraint
// (or any unique constraint when constraint is empty).
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return constraint == "" || pgErr.ConstraintName == constraint
	}
	return false
}

// IsForeignKeyViolation reports whether err is an FK violation.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
