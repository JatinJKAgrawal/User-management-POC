package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

// RoleService manages tenant roles and their permissions.
type RoleService struct{ d Deps }

func (s *RoleService) ListModules(ctx context.Context) ([]domain.Module, error) {
	return s.d.Store.Permissions.ListModules(ctx)
}

func (s *RoleService) List(ctx context.Context, actor *domain.Principal) ([]domain.Role, error) {
	return s.d.Store.Roles.List(ctx, actor.TenantID)
}

func (s *RoleService) Get(ctx context.Context, actor *domain.Principal, id uuid.UUID) (*domain.Role, error) {
	r, err := s.d.Store.Roles.GetByID(ctx, actor.TenantID, id)
	return r, mapNotFound(err, "role_not_found", "role not found")
}

type RoleInput struct {
	Name        string
	Description string
	Permissions []string // permission codes, e.g. "inventory:read"
}

func (s *RoleService) Create(ctx context.Context, actor *domain.Principal, in RoleInput) (*domain.Role, error) {
	name := strings.TrimSpace(in.Name)
	if strings.EqualFold(name, domain.TenantAdminRoleName) {
		return nil, apperror.Conflict("role_exists", "role name is reserved")
	}
	var id uuid.UUID
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		permIDs, err := s.resolvePermissions(ctx, tx, actor, in.Permissions)
		if err != nil {
			return err
		}
		id, err = tx.Roles.Create(ctx, actor.TenantID, name, strings.TrimSpace(in.Description), false)
		if err != nil {
			if repository.IsUniqueViolation(err, "uq_roles_tenant_name") {
				return apperror.Conflict("role_exists", "a role with this name already exists")
			}
			return err
		}
		return tx.Roles.ReplacePermissions(ctx, id, permIDs)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, actor, id)
}

func (s *RoleService) Update(ctx context.Context, actor *domain.Principal, id uuid.UUID, name, description string) (*domain.Role, error) {
	name = strings.TrimSpace(name)
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		if _, err := s.mutableRole(ctx, tx, actor, id); err != nil {
			return err
		}
		if strings.EqualFold(name, domain.TenantAdminRoleName) {
			return apperror.Conflict("role_exists", "role name is reserved")
		}
		if err := tx.Roles.Update(ctx, actor.TenantID, id, name, strings.TrimSpace(description)); err != nil {
			if repository.IsUniqueViolation(err, "uq_roles_tenant_name") {
				return apperror.Conflict("role_exists", "a role with this name already exists")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, actor, id)
}

// SetPermissions replaces the permission set of a role.
func (s *RoleService) SetPermissions(ctx context.Context, actor *domain.Principal, id uuid.UUID, codes []string) (*domain.Role, error) {
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		if _, err := s.mutableRole(ctx, tx, actor, id); err != nil {
			return err
		}
		permIDs, err := s.resolvePermissions(ctx, tx, actor, codes)
		if err != nil {
			return err
		}
		return tx.Roles.ReplacePermissions(ctx, id, permIDs)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, actor, id)
}

func (s *RoleService) Delete(ctx context.Context, actor *domain.Principal, id uuid.UUID) error {
	return s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		if _, err := s.mutableRole(ctx, tx, actor, id); err != nil {
			return err
		}
		n, err := tx.Roles.CountUsers(ctx, actor.TenantID, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return apperror.Conflict("role_in_use", "role is assigned to users; unassign it first").
				WithDetails(map[string]any{"assigned_users": n})
		}
		return tx.Roles.Delete(ctx, actor.TenantID, id)
	})
}

// ---------------------------------------------------------------------------

func (s *RoleService) mutableRole(ctx context.Context, tx *repository.Store, actor *domain.Principal, id uuid.UUID) (*domain.Role, error) {
	r, err := tx.Roles.GetByID(ctx, actor.TenantID, id)
	if err != nil {
		return nil, mapNotFound(err, "role_not_found", "role not found")
	}
	if r.IsSystem {
		return nil, apperror.Forbidden("system_role_immutable", "system roles cannot be modified")
	}
	// Editing a role changes the access of everyone who holds it, so the
	// caller must already hold every permission currently in it.
	if err := ensureCanGrant(actor, nil, r.Permissions); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *RoleService) resolvePermissions(ctx context.Context, tx *repository.Store, actor *domain.Principal, codes []string) ([]uuid.UUID, error) {
	codes = dedupeStrings(codes)
	if len(codes) == 0 {
		return nil, nil
	}
	perms, err := tx.Permissions.FindByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	if len(perms) != len(codes) {
		found := make(map[string]bool, len(perms))
		for _, p := range perms {
			found[p.Code] = true
		}
		var unknown []string
		for _, c := range codes {
			if !found[c] {
				unknown = append(unknown, c)
			}
		}
		return nil, apperror.Validation("unknown_permissions", "one or more permissions do not exist").
			WithDetails(map[string]any{"permissions": unknown})
	}
	if err := ensureCanGrant(actor, nil, codes); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(perms))
	for i, p := range perms {
		ids[i] = p.ID
	}
	return ids, nil
}
