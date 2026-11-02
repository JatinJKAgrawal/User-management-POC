// Package middleware contains gin middleware for cross-cutting concerns:
// request IDs, logging, recovery, authentication and RBAC authorisation.
package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/httpx"
)

const principalKey = "principal"

// RequestID propagates X-Request-ID or generates one.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" || len(id) > 128 {
			id = uuid.NewString()
		}
		c.Set(httpx.RequestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// Logger emits one structured log line per request.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", c.GetString(httpx.RequestIDKey),
		}
		if p, ok := c.Get(principalKey); ok {
			pr := p.(*domain.Principal)
			attrs = append(attrs, "tenant_id", pr.TenantID, "user_id", pr.UserID)
		}
		switch s := c.Writer.Status(); {
		case s >= 500:
			log.Error("http request", attrs...)
		case s >= 400:
			log.Warn("http request", attrs...)
		default:
			log.Info("http request", attrs...)
		}
	}
}

// Recovery converts panics into a 500 JSON response.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, rec any) {
		log.Error("panic recovered", "panic", rec, "request_id", c.GetString(httpx.RequestIDKey))
		httpx.Error(c, apperror.Internal(nil))
	})
}

// SecurityHeaders sets conservative headers for a JSON API.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		c.Next()
	}
}

// RequireAuth validates the Bearer access token and stores the principal.
func RequireAuth(authenticate func(c *gin.Context, token string) (*domain.Principal, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		scheme, token, ok := strings.Cut(h, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			c.Header("WWW-Authenticate", `Bearer realm="api"`)
			httpx.Error(c, apperror.Unauthorized("missing_token", "missing or malformed Authorization header"))
			return
		}
		p, err := authenticate(c, strings.TrimSpace(token))
		if err != nil {
			c.Header("WWW-Authenticate", `Bearer realm="api", error="invalid_token"`)
			httpx.Error(c, err)
			return
		}
		c.Set(principalKey, p)
		c.Next()
	}
}

// RequirePermission enforces RBAC: the caller must hold every listed permission.
func RequirePermission(codes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := MustPrincipal(c)
		for _, code := range codes {
			if !p.Can(code) {
				httpx.Error(c, apperror.Forbidden("permission_denied", "you do not have permission to perform this action").
					WithDetails(map[string]string{"required": code}))
				return
			}
		}
		c.Next()
	}
}

// PlatformAPIKey protects super-admin endpoints with a static key.
func PlatformAPIKey(key string) gin.HandlerFunc {
	k := []byte(key)
	return func(c *gin.Context) {
		got := []byte(c.GetHeader("X-Platform-API-Key"))
		if len(got) == 0 || subtle.ConstantTimeCompare(got, k) != 1 {
			httpx.Error(c, apperror.Unauthorized("invalid_platform_key", "invalid or missing platform API key"))
			return
		}
		c.Next()
	}
}

// MustPrincipal returns the authenticated principal. It must only be used on
// routes behind RequireAuth.
func MustPrincipal(c *gin.Context) *domain.Principal {
	v, ok := c.Get(principalKey)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		panic("MustPrincipal used on unauthenticated route")
	}
	return v.(*domain.Principal)
}
