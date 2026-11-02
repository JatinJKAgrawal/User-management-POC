// Package domain holds the core entities shared across layers.
package domain

import (
	"time"

	"github.com/google/uuid"
)

type TenantStatus string

const (
	TenantActive    TenantStatus = "active"
	TenantSuspended TenantStatus = "suspended"
)

type Tenant struct {
	ID        uuid.UUID    `json:"id"`
	Name      string       `json:"name"`
	Slug      string       `json:"slug"`
	Status    TenantStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type UserStatus string

const (
	// UserInvited: invitation email sent, email not yet verified.
	UserInvited UserStatus = "invited"
	// UserPendingPassword: email verified via onboarding link, password not set.
	UserPendingPassword UserStatus = "pending_password"
	// UserActive: can log in.
	UserActive UserStatus = "active"
	// UserDisabled: blocked by an administrator.
	UserDisabled UserStatus = "disabled"
)

type User struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	Email           string     `json:"email"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	PasswordHash    *string    `json:"-"`
	Status          UserStatus `json:"status"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	PasswordSetAt   *time.Time `json:"password_set_at,omitempty"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	InvitedBy       *uuid.UUID `json:"invited_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Roles           []RoleRef  `json:"roles"`
}

func (u *User) FullName() string {
	switch {
	case u.FirstName != "" && u.LastName != "":
		return u.FirstName + " " + u.LastName
	case u.FirstName != "":
		return u.FirstName
	default:
		return u.Email
	}
}

type Module struct {
	ID          uuid.UUID    `json:"id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
}

type Permission struct {
	ID          uuid.UUID `json:"id"`
	ModuleCode  string    `json:"module"`
	Action      string    `json:"action"`
	Code        string    `json:"code"`
	Description string    `json:"description"`
}

type Role struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RoleRef struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	IsSystem bool      `json:"is_system"`
}

type ActionTokenPurpose string

const PurposeOnboarding ActionTokenPurpose = "onboarding"

type ActionToken struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	Purpose   ActionTokenPurpose
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type RefreshSession struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
	UserAgent string
	IPAddress string
}

type InventoryItem struct {
	ID        uuid.UUID  `json:"id"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	SKU       string     `json:"sku"`
	Name      string     `json:"name"`
	Quantity  int        `json:"quantity"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Principal is the authenticated caller, resolved from the access token and
// the database on every request.
type Principal struct {
	UserID      uuid.UUID
	TenantID    uuid.UUID
	Email       string
	IsAdmin     bool // holds a system (Tenant Admin) role
	Permissions map[string]struct{}
}

func (p *Principal) Can(code string) bool {
	_, ok := p.Permissions[code]
	return ok
}

// Pagination is shared by list endpoints.
type Page struct {
	Limit  int
	Offset int
}

const TenantAdminRoleName = "Tenant Admin"
