package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
)

type InventoryRepo struct{ db DBTX }

const inventoryCols = `id, tenant_id, sku, name, quantity, created_by, created_at, updated_at`

func scanItem(row pgx.Row) (*domain.InventoryItem, error) {
	var i domain.InventoryItem
	if err := row.Scan(&i.ID, &i.TenantID, &i.SKU, &i.Name, &i.Quantity, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &i, nil
}

func (r *InventoryRepo) Create(ctx context.Context, tenantID uuid.UUID, sku, name string, qty int, createdBy uuid.UUID) (*domain.InventoryItem, error) {
	return scanItem(r.db.QueryRow(ctx, `
		INSERT INTO inventory_items (tenant_id, sku, name, quantity, created_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+inventoryCols, tenantID, sku, name, qty, createdBy))
}

func (r *InventoryRepo) Get(ctx context.Context, tenantID, id uuid.UUID) (*domain.InventoryItem, error) {
	return scanItem(r.db.QueryRow(ctx,
		`SELECT `+inventoryCols+` FROM inventory_items WHERE tenant_id = $1 AND id = $2`, tenantID, id))
}

func (r *InventoryRepo) List(ctx context.Context, tenantID uuid.UUID, p domain.Page) ([]domain.InventoryItem, int, error) {
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM inventory_items WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+inventoryCols+` FROM inventory_items WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, tenantID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.InventoryItem{}
	for rows.Next() {
		i, err := scanItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *i)
	}
	return out, total, rows.Err()
}

func (r *InventoryRepo) Update(ctx context.Context, tenantID, id uuid.UUID, name string, qty int) (*domain.InventoryItem, error) {
	return scanItem(r.db.QueryRow(ctx, `
		UPDATE inventory_items SET name = $3, quantity = $4
		WHERE tenant_id = $1 AND id = $2 RETURNING `+inventoryCols, tenantID, id, name, qty))
}

func (r *InventoryRepo) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM inventory_items WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type InventoryStats struct {
	Items         int `json:"items"`
	TotalQuantity int `json:"total_quantity"`
	OutOfStock    int `json:"out_of_stock"`
}

func (r *InventoryRepo) Stats(ctx context.Context, tenantID uuid.UUID) (InventoryStats, error) {
	var s InventoryStats
	err := r.db.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(quantity), 0), count(*) FILTER (WHERE quantity = 0)
		FROM inventory_items WHERE tenant_id = $1`, tenantID).Scan(&s.Items, &s.TotalQuantity, &s.OutOfStock)
	return s, err
}
