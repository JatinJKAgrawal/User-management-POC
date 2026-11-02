package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

// UserService is the "user management" module's user side. Every method is
// scoped to the caller's tenant (taken from the verified JWT, never from input).
type UserService struct {
	d          Deps
	onboarding *OnboardingService
}

type InviteUserInput struct {
	Email     string
	FirstName string
	LastName  string
	RoleIDs   []uuid.UUID
}

type InviteResult struct {
	User            *domain.User `json:"user"`
	InviteEmailSent bool         `json:"invite_email_sent"`
}

// Invite creates a user in the "invited" state, assigns the requested roles
// and emails a one-time onboarding link.
func (s *UserService) Invite(ctx context.Context, actor *domain.Principal, in InviteUserInput) (*InviteResult, error) {
	email := normalizeEmail(in.Email)
	roleIDs := dedupeUUIDs(in.RoleIDs)

	var user *domain.User
	var invite *PendingInvite
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		tenant, err := tx.Tenants.GetByID(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		roles, err := s.loadRoles(ctx, tx, actor, roleIDs)
		if err != nil {
			return err
		}

		user, err = tx.Users.Create(ctx, repository.CreateUserParams{
			TenantID:  actor.TenantID,
			Email:     email,
			FirstName: strings.TrimSpace(in.FirstName),
			LastName:  strings.TrimSpace(in.LastName),
			InvitedBy: &actor.UserID,
		})
		if err != nil {
			if repository.IsUniqueViolation(err, "uq_users_tenant_email") {
				return apperror.Conflict("user_exists", "a user with this email already exists in the tenant")
			}
			return err
		}
		if err := tx.Roles.ReplaceUserRoles(ctx, actor.TenantID, user.ID, roleIDs, &actor.UserID); err != nil {
			return err
		}
		user.Roles = toRefs(roles)

		invite, err = s.onboarding.IssueInvite(ctx, tx, tenant, user)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &InviteResult{User: user, InviteEmailSent: s.onboarding.Deliver(ctx, invite)}, nil
}

// ResendInvite issues a new onboarding link (the old one stops working).
func (s *UserService) ResendInvite(ctx context.Context, actor *domain.Principal, userID uuid.UUID) (*InviteResult, error) {
	var user *domain.User
	var invite *PendingInvite
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		tenant, err := tx.Tenants.GetByID(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		user, err = tx.Users.GetByIDForUpdate(ctx, actor.TenantID, userID)
		if err != nil {
			return mapNotFound(err, "user_not_found", "user not found")
		}
		if user.Status != domain.UserInvited && user.Status != domain.UserPendingPassword {
			return apperror.Conflict("already_onboarded", "user has already completed onboarding")
		}
		invite, err = s.onboarding.IssueInvite(ctx, tx, tenant, user)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := s.fillRoles(ctx, actor.TenantID, user); err != nil {
		return nil, err
	}
	return &InviteResult{User: user, InviteEmailSent: s.onboarding.Deliver(ctx, invite)}, nil
}

func (s *UserService) List(ctx context.Context, actor *domain.Principal, f repository.ListUsersFilter) ([]domain.User, int, error) {
	users, total, err := s.d.Store.Users.List(ctx, actor.TenantID, f)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	roles, err := s.d.Store.Roles.RolesForUsers(ctx, actor.TenantID, ids)
	if err != nil {
		return nil, 0, err
	}
	attachRoles(roles, users)
	return users, total, nil
}

func (s *UserService) Get(ctx context.Context, actor *domain.Principal, id uuid.UUID) (*domain.User, error) {
	u, err := s.d.Store.Users.GetByID(ctx, actor.TenantID, id)
	if err != nil {
		return nil, mapNotFound(err, "user_not_found", "user not found")
	}
	return u, s.fillRoles(ctx, actor.TenantID, u)
}

func (s *UserService) UpdateProfile(ctx context.Context, actor *domain.Principal, id uuid.UUID, firstName, lastName string) (*domain.User, error) {
	u, err := s.d.Store.Users.UpdateProfile(ctx, actor.TenantID, id, strings.TrimSpace(firstName), strings.TrimSpace(lastName))
	if err != nil {
		return nil, mapNotFound(err, "user_not_found", "user not found")
	}
	return u, s.fillRoles(ctx, actor.TenantID, u)
}

// SetStatus enables or disables a user. Disabling revokes all their sessions.
func (s *UserService) SetStatus(ctx context.Context, actor *domain.Principal, id uuid.UUID, enabled bool) (*domain.User, error) {
	if id == actor.UserID {
		return nil, apperror.Forbidden("cannot_modify_self", "you cannot change your own status")
	}
	var user *domain.User
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		if _, err := tx.Tenants.Lock(ctx, actor.TenantID); err != nil {
			return err
		}
		current, err := tx.Users.GetByIDForUpdate(ctx, actor.TenantID, id)
		if err != nil {
			return mapNotFound(err, "user_not_found", "user not found")
		}

		var target domain.UserStatus
		switch {
		case enabled && current.Status == domain.UserDisabled:
			if current.PasswordSetAt == nil {
				target = domain.UserInvited
				if current.EmailVerifiedAt != nil {
					target = domain.UserPendingPassword
				}
			} else {
				target = domain.UserActive
			}
		case enabled:
			target = current.Status // already enabled; no-op
		default:
			if err := s.guardLastAdmin(ctx, tx, actor.TenantID, id); err != nil {
				return err
			}
			target = domain.UserDisabled
		}

		if user, err = tx.Users.UpdateStatus(ctx, actor.TenantID, id, target); err != nil {
			return err
		}
		if target == domain.UserDisabled {
			if err := tx.Tokens.RevokeAllForUser(ctx, actor.TenantID, id); err != nil {
				return err
			}
			return tx.Tokens.InvalidateActionTokens(ctx, actor.TenantID, id, domain.PurposeOnboarding)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return user, s.fillRoles(ctx, actor.TenantID, user)
}

// AssignRoles replaces the user's roles with the given set.
func (s *UserService) AssignRoles(ctx context.Context, actor *domain.Principal, id uuid.UUID, roleIDs []uuid.UUID) (*domain.User, error) {
	roleIDs = dedupeUUIDs(roleIDs)
	var user *domain.User
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		if _, err := tx.Tenants.Lock(ctx, actor.TenantID); err != nil {
			return err
		}
		var err error
		user, err = tx.Users.GetByIDForUpdate(ctx, actor.TenantID, id)
		if err != nil {
			return mapNotFound(err, "user_not_found", "user not found")
		}
		roles, err := s.loadRoles(ctx, tx, actor, roleIDs)
		if err != nil {
			return err
		}

		// Removing roles a user currently holds is also a privileged
		// operation: you may not strip a system role unless you are an admin.
		currentRoles, err := tx.Roles.RolesForUsers(ctx, actor.TenantID, []uuid.UUID{id})
		if err != nil {
			return err
		}
		hadAdmin, keepsAdmin := false, false
		for _, r := range currentRoles[id] {
			hadAdmin = hadAdmin || r.IsSystem
		}
		for _, r := range roles {
			keepsAdmin = keepsAdmin || r.IsSystem
		}
		if hadAdmin && !keepsAdmin {
			if !actor.IsAdmin {
				return apperror.Forbidden("cannot_revoke_system_role", "only tenant admins can revoke the admin role")
			}
			if err := s.guardLastAdmin(ctx, tx, actor.TenantID, id); err != nil {
				return err
			}
		}

		if err := tx.Roles.ReplaceUserRoles(ctx, actor.TenantID, id, roleIDs, &actor.UserID); err != nil {
			return err
		}
		user.Roles = toRefs(roles)
		return nil
	})
	return user, err
}

func (s *UserService) Delete(ctx context.Context, actor *domain.Principal, id uuid.UUID) error {
	if id == actor.UserID {
		return apperror.Forbidden("cannot_modify_self", "you cannot delete yourself")
	}
	return s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		if _, err := tx.Tenants.Lock(ctx, actor.TenantID); err != nil {
			return err
		}
		if _, err := tx.Users.GetByIDForUpdate(ctx, actor.TenantID, id); err != nil {
			return mapNotFound(err, "user_not_found", "user not found")
		}
		if err := s.guardLastAdmin(ctx, tx, actor.TenantID, id); err != nil {
			return err
		}
		return tx.Users.Delete(ctx, actor.TenantID, id)
	})
}

// ---------------------------------------------------------------------------

// loadRoles verifies that every role ID belongs to the caller's tenant and
// that granting them would not escalate the caller's privileges.
func (s *UserService) loadRoles(ctx context.Context, tx *repository.Store, actor *domain.Principal, ids []uuid.UUID) ([]domain.Role, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	roles, err := tx.Roles.ListByIDs(ctx, actor.TenantID, ids)
	if err != nil {
		return nil, err
	}
	if len(roles) != len(ids) {
		found := make(map[uuid.UUID]bool, len(roles))
		for _, r := range roles {
			found[r.ID] = true
		}
		var missing []uuid.UUID
		for _, id := range ids {
			if !found[id] {
				missing = append(missing, id)
			}
		}
		return nil, apperror.Validation("unknown_roles", "one or more roles do not exist").
			WithDetails(map[string]any{"role_ids": missing})
	}
	return roles, ensureCanGrant(actor, roles, nil)
}

// guardLastAdmin fails if userID is currently an admin and no other active
// admin would remain.
func (s *UserService) guardLastAdmin(ctx context.Context, tx *repository.Store, tenantID, userID uuid.UUID) error {
	roles, err := tx.Roles.RolesForUsers(ctx, tenantID, []uuid.UUID{userID})
	if err != nil {
		return err
	}
	isAdmin := false
	for _, r := range roles[userID] {
		isAdmin = isAdmin || r.IsSystem
	}
	if !isAdmin {
		return nil
	}
	others, err := tx.Roles.CountActiveAdmins(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if others == 0 {
		return apperror.Conflict("last_admin", "the tenant must keep at least one active administrator")
	}
	return nil
}

func (s *UserService) fillRoles(ctx context.Context, tenantID uuid.UUID, u *domain.User) error {
	roles, err := s.d.Store.Roles.RolesForUsers(ctx, tenantID, []uuid.UUID{u.ID})
	if err != nil {
		return err
	}
	if r := roles[u.ID]; r != nil {
		u.Roles = r
	} else {
		u.Roles = []domain.RoleRef{}
	}
	return nil
}

func toRefs(roles []domain.Role) []domain.RoleRef {
	out := make([]domain.RoleRef, 0, len(roles))
	for _, r := range roles {
		out = append(out, domain.RoleRef{ID: r.ID, Name: r.Name, IsSystem: r.IsSystem})
	}
	return out
}
