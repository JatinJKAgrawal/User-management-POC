// Package service implements the business logic. Services are transport
// agnostic: they accept plain inputs and return domain objects or
// *apperror.Error values.
package service

import (
	"log/slog"

	"github.com/jatinjkagrawal/user-management-poc/internal/auth"
	"github.com/jatinjkagrawal/user-management-poc/internal/config"
	"github.com/jatinjkagrawal/user-management-poc/internal/platform/mailer"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

type Deps struct {
	Store  *repository.Store
	JWT    *auth.JWTManager
	Mailer mailer.Mailer
	Config *config.Config
	Log    *slog.Logger
}

type Services struct {
	Tenants    *TenantService
	Auth       *AuthService
	Users      *UserService
	Roles      *RoleService
	Inventory  *InventoryService
	Reports    *ReportService
	Billing    *BillingService
	Onboarding *OnboardingService
}

func New(d Deps) *Services {
	onboarding := &OnboardingService{d: d}
	return &Services{
		Onboarding: onboarding,
		Tenants:    &TenantService{d: d, onboarding: onboarding},
		Auth:       &AuthService{d: d},
		Users:      &UserService{d: d, onboarding: onboarding},
		Roles:      &RoleService{d: d},
		Inventory:  &InventoryService{d: d},
		Reports:    &ReportService{d: d},
		Billing:    &BillingService{},
	}
}
