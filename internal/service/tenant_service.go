package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

// TenantService provisions tenants. It is only reachable through the platform
// (super-admin) API.
type TenantService struct {
	d          Deps
	onboarding *OnboardingService
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type CreateTenantInput struct {
	Name           string
	Slug           string
	AdminEmail     string
	AdminFirstName string
	AdminLastName  string
}

type CreateTenantResult struct {
	Tenant          *domain.Tenant `json:"tenant"`
	Admin           *domain.User   `json:"admin"`
	InviteEmailSent bool           `json:"invite_email_sent"`
}

// Create provisions a tenant, its immutable "Tenant Admin" system role holding
// every permission, and invites the first administrator by email.
func (s *TenantService) Create(ctx context.Context, in CreateTenantInput) (*CreateTenantResult, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugRe.MatchString(in.Slug) {
		return nil, apperror.Validation("invalid_slug", "slug must contain only lowercase letters, digits and single dashes")
	}

	var res CreateTenantResult
	var invite *PendingInvite
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		tenant, err := tx.Tenants.Create(ctx, strings.TrimSpace(in.Name), in.Slug)
		if err != nil {
			if repository.IsUniqueViolation(err, "") {
				return apperror.Conflict("tenant_exists", "a tenant with this slug already exists")
			}
			return err
		}

		roleID, err := tx.Roles.Create(ctx, tenant.ID, domain.TenantAdminRoleName,
			"Built-in role with full access to every module", true)
		if err != nil {
			return err
		}
		permIDs, err := tx.Permissions.AllIDs(ctx)
		if err != nil {
			return err
		}
		if err := tx.Roles.ReplacePermissions(ctx, roleID, permIDs); err != nil {
			return err
		}

		admin, err := tx.Users.Create(ctx, repository.CreateUserParams{
			TenantID:  tenant.ID,
			Email:     normalizeEmail(in.AdminEmail),
			FirstName: strings.TrimSpace(in.AdminFirstName),
			LastName:  strings.TrimSpace(in.AdminLastName),
		})
		if err != nil {
			return err
		}
		if err := tx.Roles.ReplaceUserRoles(ctx, tenant.ID, admin.ID, []uuid.UUID{roleID}, nil); err != nil {
			return err
		}
		admin.Roles = []domain.RoleRef{{ID: roleID, Name: domain.TenantAdminRoleName, IsSystem: true}}

		if invite, err = s.onboarding.IssueInvite(ctx, tx, tenant, admin); err != nil {
			return err
		}
		res.Tenant, res.Admin = tenant, admin
		return nil
	})
	if err != nil {
		return nil, err
	}
	res.InviteEmailSent = s.onboarding.Deliver(ctx, invite)
	return &res, nil
}

func (s *TenantService) List(ctx context.Context, p domain.Page) ([]domain.Tenant, int, error) {
	return s.d.Store.Tenants.List(ctx, p)
}

func (s *TenantService) Get(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	t, err := s.d.Store.Tenants.GetByID(ctx, id)
	return t, mapNotFound(err, "tenant_not_found", "tenant not found")
}

func (s *TenantService) SetStatus(ctx context.Context, id uuid.UUID, status domain.TenantStatus) (*domain.Tenant, error) {
	if status != domain.TenantActive && status != domain.TenantSuspended {
		return nil, apperror.Validation("invalid_status", "status must be 'active' or 'suspended'")
	}
	t, err := s.d.Store.Tenants.UpdateStatus(ctx, id, status)
	return t, mapNotFound(err, "tenant_not_found", "tenant not found")
}
