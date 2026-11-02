// Package server wires dependencies into the gin router and runs the HTTP server.
package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/config"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/handler"
	"github.com/jatinjkagrawal/user-management-poc/internal/httpx"
	"github.com/jatinjkagrawal/user-management-poc/internal/middleware"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
	"github.com/jatinjkagrawal/user-management-poc/internal/service"
)

func NewRouter(cfg *config.Config, log *slog.Logger, store *repository.Store, svc *service.Services) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	_ = r.SetTrustedProxies(cfg.HTTP.TrustedProxies)
	r.HandleMethodNotAllowed = true
	r.Use(middleware.RequestID(), middleware.Recovery(log), middleware.Logger(log), middleware.SecurityHeaders())
	r.NoRoute(func(c *gin.Context) { httpx.Error(c, apperror.NotFound("route_not_found", "route not found")) })
	r.NoMethod(func(c *gin.Context) {
		httpx.Error(c, &apperror.Error{Kind: apperror.KindValidation, Code: "method_not_allowed", Message: "method not allowed"})
	})

	health := handler.NewHealthHandler(store)
	r.GET("/healthz", health.Live)
	r.GET("/readyz", health.Ready)

	var (
		authH     = handler.NewAuthHandler(svc.Auth, cfg.Onboard.SetPasswordURL)
		platformH = handler.NewPlatformHandler(svc.Tenants)
		userH     = handler.NewUserHandler(svc.Users)
		roleH     = handler.NewRoleHandler(svc.Roles)
		invH      = handler.NewInventoryHandler(svc.Inventory)
		repH      = handler.NewReportHandler(svc.Reports)
		billH     = handler.NewBillingHandler(svc.Billing)
		perm      = middleware.RequirePermission
	)

	v1 := r.Group("/api/v1")

	// ---- Platform (super admin): tenant provisioning ----------------------
	platform := v1.Group("/platform", middleware.PlatformAPIKey(cfg.Platform.APIKey))
	platform.POST("/tenants", platformH.CreateTenant)
	platform.GET("/tenants", platformH.ListTenants)
	platform.GET("/tenants/:id", platformH.GetTenant)
	platform.PATCH("/tenants/:id/status", platformH.SetTenantStatus)

	// ---- Public auth endpoints --------------------------------------------
	pub := v1.Group("/auth")
	pub.GET("/onboarding/verify", authH.VerifyOnboarding)
	pub.POST("/onboarding/set-password", authH.SetPassword)
	pub.POST("/login", authH.Login)
	pub.POST("/refresh", authH.Refresh)
	pub.POST("/logout", authH.Logout)

	// ---- Authenticated (tenant scoped via JWT) -----------------------------
	authed := v1.Group("", middleware.RequireAuth(func(c *gin.Context, token string) (*domain.Principal, error) {
		return svc.Auth.Authenticate(c.Request.Context(), token)
	}))
	authed.GET("/auth/me", authH.Me)
	authed.POST("/auth/logout-all", authH.LogoutAll)

	// User management module
	authed.GET("/modules", perm(domain.PermUserMgmtRead), roleH.ListModules)

	users := authed.Group("/users")
	users.GET("", perm(domain.PermUserMgmtRead), userH.List)
	users.POST("", perm(domain.PermUserMgmtCreate), userH.Invite)
	users.GET("/:id", perm(domain.PermUserMgmtRead), userH.Get)
	users.PATCH("/:id", perm(domain.PermUserMgmtUpdate), userH.Update)
	users.PATCH("/:id/status", perm(domain.PermUserMgmtUpdate), userH.SetStatus)
	users.PUT("/:id/roles", perm(domain.PermUserMgmtUpdate), userH.AssignRoles)
	users.POST("/:id/resend-invite", perm(domain.PermUserMgmtCreate), userH.ResendInvite)
	users.DELETE("/:id", perm(domain.PermUserMgmtDelete), userH.Delete)

	roles := authed.Group("/roles")
	roles.GET("", perm(domain.PermUserMgmtRead), roleH.List)
	roles.POST("", perm(domain.PermUserMgmtCreate), roleH.Create)
	roles.GET("/:id", perm(domain.PermUserMgmtRead), roleH.Get)
	roles.PUT("/:id", perm(domain.PermUserMgmtUpdate), roleH.Update)
	roles.PUT("/:id/permissions", perm(domain.PermUserMgmtUpdate), roleH.SetPermissions)
	roles.DELETE("/:id", perm(domain.PermUserMgmtDelete), roleH.Delete)

	// Inventory module (dummy)
	inv := authed.Group("/inventory/items")
	inv.GET("", perm(domain.PermInventoryRead), invH.List)
	inv.POST("", perm(domain.PermInventoryCreate), invH.Create)
	inv.GET("/:id", perm(domain.PermInventoryRead), invH.Get)
	inv.PUT("/:id", perm(domain.PermInventoryUpdate), invH.Update)
	inv.DELETE("/:id", perm(domain.PermInventoryDelete), invH.Delete)

	// Reports module (dummy)
	authed.GET("/reports/summary", perm(domain.PermReportsRead), repH.Summary)
	authed.GET("/reports/inventory.csv", perm(domain.PermReportsExport), repH.ExportInventory)

	// Billing module (dummy)
	authed.GET("/billing/invoices", perm(domain.PermBillingRead), billH.Invoices)
	authed.PUT("/billing/subscription", perm(domain.PermBillingManage), billH.ChangePlan)

	return r
}
