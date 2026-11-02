package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
)

type RoleRepo struct{ db DBTX }

// roleSelect returns each role with its permission codes aggregated.
const roleSelect = `
	SELECT r.id, r.tenant_id, r.name, r.description, r.is_system, r.created_at, r.updated_at,
	       COALESCE(array_agg(p.code ORDER BY p.code) FILTER (WHERE p.code IS NOT NULL), '{}') AS permissions
	FROM roles r
	LEFT JOIN role_permissions rp ON rp.role_id = r.id
	LEFT JOIN permissions p ON p.id = rp.permission_id`

func (r *RoleRepo) queryRoles(ctx context.Context, sql string, args ...any) ([]domain.Role, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Role{}
	for rows.Next() {
		var ro domain.Role
		if err := rows.Scan(&ro.ID, &ro.TenantID, &ro.Name, &ro.Description, &ro.IsSystem,
			&ro.CreatedAt, &ro.UpdatedAt, &ro.Permissions); err != nil {
			return nil, err
		}
		out = append(out, ro)
	}
	return out, rows.Err()
}

func (r *RoleRepo) Create(ctx context.Context, tenantID uuid.UUID, name, description string, isSystem bool) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO roles (tenant_id, name, description, is_system) VALUES ($1, $2, $3, $4) RETURNING id`,
		tenantID, name, description, isSystem).Scan(&id)
	return id, err
}

func (r *RoleRepo) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.Role, error) {
	roles, err := r.queryRoles(ctx, roleSelect+` WHERE r.tenant_id = $1 AND r.id = $2 GROUP BY r.id`, tenantID, id)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, ErrNotFound
	}
	return &roles[0], nil
}

func (r *RoleRepo) List(ctx context.Context, tenantID uuid.UUID) ([]domain.Role, error) {
	return r.queryRoles(ctx, roleSelect+` WHERE r.tenant_id = $1 GROUP BY r.id ORDER BY r.is_system DESC, r.name`, tenantID)
}

// ListByIDs returns the subset of ids that are roles of the tenant.
func (r *RoleRepo) ListByIDs(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) ([]domain.Role, error) {
	return r.queryRoles(ctx, roleSelect+` WHERE r.tenant_id = $1 AND r.id = ANY($2) GROUP BY r.id`, tenantID, ids)
}

func (r *RoleRepo) Update(ctx context.Context, tenantID, id uuid.UUID, name, description string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE roles SET name = $3, description = $4 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id, name, description)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *RoleRepo) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM roles WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplacePermissions sets the exact permission set of a role. The role must
// already have been verified to belong to the caller's tenant.
func (r *RoleRepo) ReplacePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	if len(permissionIDs) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id)
		SELECT $1, unnest($2::uuid[]) ON CONFLICT DO NOTHING`, roleID, permissionIDs)
	return err
}

// ReplaceUserRoles sets the exact role set of a user. The composite FKs on
// user_roles make cross-tenant assignments impossible at the DB level.
func (r *RoleRepo) ReplaceUserRoles(ctx context.Context, tenantID, userID uuid.UUID, roleIDs []uuid.UUID, assignedBy *uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM user_roles WHERE tenant_id = $1 AND user_id = $2`, tenantID, userID); err != nil {
		return err
	}
	if len(roleIDs) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_roles (tenant_id, user_id, role_id, assigned_by)
		SELECT $1, $2, unnest($3::uuid[]), $4 ON CONFLICT DO NOTHING`, tenantID, userID, roleIDs, assignedBy)
	return err
}

// RolesForUsers returns role references keyed by user ID.
func (r *RoleRepo) RolesForUsers(ctx context.Context, tenantID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID][]domain.RoleRef, error) {
	out := map[uuid.UUID][]domain.RoleRef{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT ur.user_id, r.id, r.name, r.is_system
		FROM user_roles ur JOIN roles r ON r.id = ur.role_id AND r.tenant_id = ur.tenant_id
		WHERE ur.tenant_id = $1 AND ur.user_id = ANY($2)
		ORDER BY r.name`, tenantID, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid uuid.UUID
		var ref domain.RoleRef
		if err := rows.Scan(&uid, &ref.ID, &ref.Name, &ref.IsSystem); err != nil {
			return nil, err
		}
		out[uid] = append(out[uid], ref)
	}
	return out, rows.Err()
}

// CountActiveAdmins counts active users holding a system role, excluding one
// user. Used to make sure a tenant never loses its last administrator.
func (r *RoleRepo) CountActiveAdmins(ctx context.Context, tenantID, excludeUserID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(DISTINCT u.id)
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id AND ur.tenant_id = u.tenant_id
		JOIN roles r ON r.id = ur.role_id AND r.is_system
		WHERE u.tenant_id = $1 AND u.status = 'active' AND u.id <> $2`, tenantID, excludeUserID).Scan(&n)
	return n, err
}

func (r *RoleRepo) CountUsers(ctx context.Context, tenantID, roleID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE tenant_id = $1 AND role_id = $2`,
		tenantID, roleID).Scan(&n)
	return n, err
}

func (r *RoleRepo) Count(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM roles WHERE tenant_id = $1`, tenantID).Scan(&n)
	return n, err
}
