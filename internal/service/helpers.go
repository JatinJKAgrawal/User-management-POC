package service

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// mapNotFound converts repository.ErrNotFound into a typed 404.
func mapNotFound(err error, code, msg string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return apperror.NotFound(code, msg)
	}
	return err
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if _, ok := seen[s]; !ok && s != "" {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// ensureCanGrant prevents privilege escalation: a caller can only grant
// permissions it holds itself, and only tenant admins can grant system roles.
func ensureCanGrant(actor *domain.Principal, roles []domain.Role, codes []string) error {
	var missing []string
	for _, r := range roles {
		if r.IsSystem && !actor.IsAdmin {
			return apperror.Forbidden("cannot_grant_system_role", "only tenant admins can assign the "+r.Name+" role")
		}
		for _, c := range r.Permissions {
			if !actor.Can(c) {
				missing = append(missing, c)
			}
		}
	}
	for _, c := range codes {
		if !actor.Can(c) {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return apperror.Forbidden("privilege_escalation",
			"you cannot grant permissions you do not hold").WithDetails(map[string]any{"permissions": dedupeStrings(missing)})
	}
	return nil
}

// attachRoles fills the Roles field of the given users.
func attachRoles(rolesByUser map[uuid.UUID][]domain.RoleRef, users []domain.User) {
	for i := range users {
		if r, ok := rolesByUser[users[i].ID]; ok {
			users[i].Roles = r
		} else {
			users[i].Roles = []domain.RoleRef{}
		}
	}
}
