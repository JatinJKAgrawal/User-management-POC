package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
)

type PermissionRepo struct{ db DBTX }

func (r *PermissionRepo) ListModules(ctx context.Context) ([]domain.Module, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.id, m.code, m.name, m.description,
		       p.id, p.action, p.code, p.description
		FROM modules m
		LEFT JOIN permissions p ON p.module_id = m.id
		ORDER BY m.code, p.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Module{}
	idx := map[uuid.UUID]int{}
	for rows.Next() {
		var m domain.Module
		var pid *uuid.UUID
		var action, code, desc *string
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &m.Description, &pid, &action, &code, &desc); err != nil {
			return nil, err
		}
		i, ok := idx[m.ID]
		if !ok {
			m.Permissions = []domain.Permission{}
			out = append(out, m)
			i = len(out) - 1
			idx[m.ID] = i
		}
		if pid != nil {
			out[i].Permissions = append(out[i].Permissions, domain.Permission{
				ID: *pid, ModuleCode: m.Code, Action: *action, Code: *code, Description: *desc,
			})
		}
	}
	return out, rows.Err()
}

// FindByCodes returns the permissions matching the given codes.
func (r *PermissionRepo) FindByCodes(ctx context.Context, codes []string) ([]domain.Permission, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, m.code, p.action, p.code, p.description
		FROM permissions p JOIN modules m ON m.id = p.module_id
		WHERE p.code = ANY($1)`, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Permission{}
	for rows.Next() {
		var p domain.Permission
		if err := rows.Scan(&p.ID, &p.ModuleCode, &p.Action, &p.Code, &p.Description); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PermissionRepo) AllIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `SELECT id FROM permissions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ForUser returns the effective permission codes of a user and whether the
// user holds a system (admin) role.
func (r *PermissionRepo) ForUser(ctx context.Context, tenantID, userID uuid.UUID) (codes []string, isAdmin bool, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(array_agg(DISTINCT p.code) FILTER (WHERE p.code IS NOT NULL), '{}'),
		       COALESCE(bool_or(r.is_system), false)
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id AND r.tenant_id = ur.tenant_id
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		LEFT JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.tenant_id = $1 AND ur.user_id = $2`, tenantID, userID).Scan(&codes, &isAdmin)
	return codes, isAdmin, err
}
