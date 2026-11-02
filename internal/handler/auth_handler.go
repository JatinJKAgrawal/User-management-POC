// Package handler contains the HTTP handlers. Handlers translate HTTP into
// service calls and service results back into HTTP; they hold no business logic.
package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/httpx"
	"github.com/jatinjkagrawal/user-management-poc/internal/middleware"
	"github.com/jatinjkagrawal/user-management-poc/internal/service"
)

type AuthHandler struct {
	auth           *service.AuthService
	setPasswordURL string
}

func NewAuthHandler(a *service.AuthService, setPasswordURL string) *AuthHandler {
	return &AuthHandler{auth: a, setPasswordURL: setPasswordURL}
}

// VerifyOnboarding handles the link from the invitation email.
//
//	GET /api/v1/auth/onboarding/verify?token=...
//
// Browsers are redirected (302) to the configured set-password page with a
// short-lived password-setup token. API clients that send
// "Accept: application/json" (or when no redirect URL is configured) get the
// setup token as JSON.
func (h *AuthHandler) VerifyOnboarding(c *gin.Context) {
	res, err := h.auth.VerifyOnboarding(c.Request.Context(), c.Query("token"))

	if h.setPasswordURL != "" && !wantsJSON(c) {
		q := url.Values{}
		if err != nil {
			q.Set("error", apperror.As(err).Code)
		} else {
			q.Set("token", res.SetupToken)
			q.Set("email", res.Email)
		}
		c.Redirect(http.StatusFound, h.setPasswordURL+"?"+q.Encode())
		return
	}
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, res)
}

type setPasswordRequest struct {
	Token           string `json:"token" binding:"required"`
	Password        string `json:"password" binding:"required,min=8,max=72"`
	ConfirmPassword string `json:"confirm_password" binding:"required,eqfield=Password"`
}

// SetPassword completes onboarding.
//
//	POST /api/v1/auth/onboarding/set-password
func (h *AuthHandler) SetPassword(c *gin.Context) {
	var req setPasswordRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.auth.SetPassword(c.Request.Context(), req.Token, req.Password); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"message": "password set successfully, you can now log in"})
}

type loginRequest struct {
	TenantSlug string `json:"tenant" binding:"required"`
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required"`
}

// Login exchanges credentials for an access/refresh JWT pair.
//
//	POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	pair, err := h.auth.Login(c.Request.Context(), req.TenantSlug, req.Email, req.Password, meta(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, pair)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh rotates the refresh token and issues a new access token.
//
//	POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	pair, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken, meta(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, pair)
}

// Logout revokes a refresh token.
//
//	POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.NoContent(c)
}

// LogoutAll revokes every refresh token of the caller.
//
//	POST /api/v1/auth/logout-all
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	if err := h.auth.LogoutAll(c.Request.Context(), middleware.MustPrincipal(c)); err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.NoContent(c)
}

// Me returns the caller's profile, tenant and effective permissions.
//
//	GET /api/v1/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	me, err := h.auth.Me(c.Request.Context(), middleware.MustPrincipal(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, me)
}

func meta(c *gin.Context) service.ClientMeta {
	return service.ClientMeta{UserAgent: c.Request.UserAgent(), IP: c.ClientIP()}
}

func wantsJSON(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/json")
}
