package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
)

type TenantRepo struct{ db DBTX }

const tenantCols = `id, name, slug, status, created_at, updated_at`

func scanTenant(row pgx.Row) (*domain.Tenant, error) {
	var t domain.Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

func (r *TenantRepo) Create(ctx context.Context, name, slug string) (*domain.Tenant, error) {
	return scanTenant(r.db.QueryRow(ctx,
		`INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING `+tenantCols, name, slug))
}

func (r *TenantRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return scanTenant(r.db.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id = $1`, id))
}

func (r *TenantRepo) GetBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return scanTenant(r.db.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE slug = $1`, slug))
}

func (r *TenantRepo) List(ctx context.Context, p domain.Page) ([]domain.Tenant, int, error) {
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM tenants`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+tenantCols+` FROM tenants ORDER BY created_at DESC LIMIT $1 OFFSET $2`, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.Tenant{}
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	return out, total, rows.Err()
}

func (r *TenantRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.TenantStatus) (*domain.Tenant, error) {
	return scanTenant(r.db.QueryRow(ctx,
		`UPDATE tenants SET status = $2 WHERE id = $1 RETURNING `+tenantCols, id, status))
}

// Lock takes a row lock on the tenant, serialising tenant-wide invariants
// (e.g. "at least one active admin") across concurrent transactions.
func (r *TenantRepo) Lock(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return scanTenant(r.db.QueryRow(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id = $1 FOR UPDATE`, id))
}
