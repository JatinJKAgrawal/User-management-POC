package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/apperror"
	"github.com/jatinjkagrawal/user-management-poc/internal/auth"
	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
)

type AuthService struct{ d Deps }

var (
	errInvalidCredentials = apperror.Unauthorized("invalid_credentials", "invalid tenant, email or password")
	errInvalidToken       = apperror.Unauthorized("invalid_token", "token is invalid or expired")
)

// ---------------------------------------------------------------------------
// Onboarding: verify email -> password setup token -> set password
// ---------------------------------------------------------------------------

type VerifyResult struct {
	SetupToken string    `json:"setup_token"`
	ExpiresAt  time.Time `json:"expires_at"`
	Email      string    `json:"email"`
}

// VerifyOnboarding consumes the one-time onboarding token from the email link,
// marks the email as verified and returns a short-lived password-setup JWT.
func (s *AuthService) VerifyOnboarding(ctx context.Context, plainToken string) (*VerifyResult, error) {
	if plainToken == "" {
		return nil, apperror.Validation("token_required", "token is required")
	}
	var res VerifyResult
	err := s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		tok, err := tx.Tokens.GetActionTokenForUpdate(ctx, domain.PurposeOnboarding, auth.HashOpaqueToken(plainToken))
		if errors.Is(err, repository.ErrNotFound) {
			return apperror.NotFound("invalid_onboarding_link", "onboarding link is invalid")
		}
		if err != nil {
			return err
		}
		if tok.UsedAt != nil {
			return apperror.Gone("onboarding_link_used", "onboarding link has already been used or was superseded")
		}
		if time.Now().After(tok.ExpiresAt) {
			return apperror.Gone("onboarding_link_expired", "onboarding link has expired, ask your administrator to resend it")
		}

		user, err := tx.Users.GetByIDForUpdate(ctx, tok.TenantID, tok.UserID)
		if err != nil {
			return mapNotFound(err, "invalid_onboarding_link", "onboarding link is invalid")
		}
		if user.Status != domain.UserInvited && user.Status != domain.UserPendingPassword {
			return apperror.Conflict("already_onboarded", "this account has already been activated")
		}
		if err := tx.Users.MarkEmailVerified(ctx, user.TenantID, user.ID); err != nil {
			return err
		}
		if err := tx.Tokens.MarkActionTokenUsed(ctx, tok.ID); err != nil {
			return err
		}

		setup, exp, err := s.d.JWT.Issue(auth.TokenPasswordSetup, user.ID, user.TenantID, "")
		if err != nil {
			return err
		}
		res = VerifyResult{SetupToken: setup, ExpiresAt: exp, Email: user.Email}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SetPassword completes onboarding using the password-setup JWT. It is only
// usable while the user is in the pending_password state, which makes the
// setup token effectively single use.
func (s *AuthService) SetPassword(ctx context.Context, setupToken, password string) error {
	claims, err := s.d.JWT.Parse(setupToken, auth.TokenPasswordSetup)
	if err != nil {
		return errInvalidToken
	}
	if err := auth.ValidatePasswordStrength(password); err != nil {
		return apperror.Validation("weak_password", err.Error())
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	userID, _ := claims.UserID()
	tenantID, _ := claims.TenantUUID()

	return s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		user, err := tx.Users.GetByIDForUpdate(ctx, tenantID, userID)
		if err != nil {
			return mapNotFound(err, "invalid_token", "token is invalid or expired")
		}
		if user.Status != domain.UserPendingPassword {
			return apperror.Conflict("password_already_set", "password has already been set for this account")
		}
		if err := tx.Users.SetPassword(ctx, tenantID, userID, hash); err != nil {
			return err
		}
		return tx.Tokens.RevokeAllForUser(ctx, tenantID, userID)
	})
}

// ---------------------------------------------------------------------------
// Login / refresh / logout
// ---------------------------------------------------------------------------

type ClientMeta struct {
	UserAgent string
	IP        string
}

type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_token_expires_at"`
	TokenType        string    `json:"token_type"`
}

func (s *AuthService) Login(ctx context.Context, tenantSlug, email, password string, meta ClientMeta) (*TokenPair, error) {
	tenant, err := s.d.Store.Tenants.GetBySlug(ctx, strings.ToLower(strings.TrimSpace(tenantSlug)))
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	var user *domain.User
	if tenant != nil {
		user, err = s.d.Store.Users.GetByEmail(ctx, tenant.ID, normalizeEmail(email))
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
	}

	// Always run bcrypt so response time does not reveal whether the tenant
	// or user exists.
	var hash *string
	if user != nil {
		hash = user.PasswordHash
	}
	if !auth.CheckPassword(hash, password) || user == nil {
		return nil, errInvalidCredentials
	}
	if tenant.Status != domain.TenantActive {
		return nil, apperror.Forbidden("tenant_suspended", "tenant is suspended")
	}
	if user.Status != domain.UserActive {
		return nil, apperror.Forbidden("user_inactive", "user account is not active")
	}

	var pair *TokenPair
	err = s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		if err := tx.Users.TouchLastLogin(ctx, tenant.ID, user.ID); err != nil {
			return err
		}
		pair, err = s.issuePair(ctx, tx, user.ID, tenant.ID, meta)
		return err
	})
	return pair, err
}

// Refresh rotates a refresh token. Presenting an already-revoked refresh
// token is treated as token theft and revokes every session of the user.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string, meta ClientMeta) (*TokenPair, error) {
	claims, err := s.d.JWT.Parse(refreshToken, auth.TokenRefresh)
	if err != nil {
		return nil, errInvalidToken
	}
	jti, err := uuid.Parse(claims.ID)
	if err != nil {
		return nil, errInvalidToken
	}
	userID, _ := claims.UserID()
	tenantID, _ := claims.TenantUUID()

	var pair *TokenPair
	reused := false
	err = s.d.Store.WithTx(ctx, func(tx *repository.Store) error {
		sess, err := tx.Tokens.GetRefreshSessionForUpdate(ctx, tenantID, jti)
		if err != nil {
			return mapNotFound(err, "invalid_token", "token is invalid or expired")
		}
		if sess.UserID != userID {
			return errInvalidToken
		}
		if sess.RevokedAt != nil {
			reused = true
			return tx.Tokens.RevokeAllForUser(ctx, tenantID, userID)
		}
		if time.Now().After(sess.ExpiresAt) {
			return errInvalidToken
		}
		if err := s.ensureActive(ctx, tx, tenantID, userID); err != nil {
			return err
		}
		newID := uuid.New()
		if err := tx.Tokens.RevokeRefreshSession(ctx, sess.ID, &newID); err != nil {
			return err
		}
		pair, err = s.issuePairWithID(ctx, tx, newID, userID, tenantID, meta)
		return err
	})
	if err != nil {
		return nil, err
	}
	if reused {
		s.d.Log.Warn("refresh token reuse detected, all sessions revoked", "user_id", userID, "tenant_id", tenantID)
		return nil, errInvalidToken
	}
	return pair, nil
}

// Logout revokes the given refresh token (idempotent).
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.d.JWT.Parse(refreshToken, auth.TokenRefresh)
	if err != nil {
		if errors.Is(err, auth.ErrExpiredToken) {
			return nil
		}
		return errInvalidToken
	}
	jti, err := uuid.Parse(claims.ID)
	if err != nil {
		return errInvalidToken
	}
	return s.d.Store.Tokens.RevokeRefreshSession(ctx, jti, nil)
}

// LogoutAll revokes all refresh tokens of the caller.
func (s *AuthService) LogoutAll(ctx context.Context, p *domain.Principal) error {
	return s.d.Store.Tokens.RevokeAllForUser(ctx, p.TenantID, p.UserID)
}

// ---------------------------------------------------------------------------
// Request authentication
// ---------------------------------------------------------------------------

// Authenticate validates an access token and resolves the caller's current
// state and permissions from the database, so disabling a user or changing
// their roles takes effect immediately rather than when the token expires.
func (s *AuthService) Authenticate(ctx context.Context, accessToken string) (*domain.Principal, error) {
	claims, err := s.d.JWT.Parse(accessToken, auth.TokenAccess)
	if err != nil {
		if errors.Is(err, auth.ErrExpiredToken) {
			return nil, apperror.Unauthorized("token_expired", "access token has expired")
		}
		return nil, errInvalidToken
	}
	userID, _ := claims.UserID()
	tenantID, _ := claims.TenantUUID()

	state, err := s.d.Store.Users.AuthState(ctx, tenantID, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errInvalidToken
		}
		return nil, err
	}
	if state.TenantStatus != domain.TenantActive {
		return nil, apperror.Forbidden("tenant_suspended", "tenant is suspended")
	}
	if state.UserStatus != domain.UserActive {
		return nil, apperror.Unauthorized("user_inactive", "user account is not active")
	}

	codes, isAdmin, err := s.d.Store.Permissions.ForUser(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}
	perms := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		perms[c] = struct{}{}
	}
	return &domain.Principal{UserID: userID, TenantID: tenantID, Email: state.Email, IsAdmin: isAdmin, Permissions: perms}, nil
}

type Me struct {
	User        *domain.User   `json:"user"`
	Tenant      *domain.Tenant `json:"tenant"`
	Permissions []string       `json:"permissions"`
}

func (s *AuthService) Me(ctx context.Context, p *domain.Principal) (*Me, error) {
	user, err := s.d.Store.Users.GetByID(ctx, p.TenantID, p.UserID)
	if err != nil {
		return nil, mapNotFound(err, "user_not_found", "user not found")
	}
	roles, err := s.d.Store.Roles.RolesForUsers(ctx, p.TenantID, []uuid.UUID{user.ID})
	if err != nil {
		return nil, err
	}
	if r := roles[user.ID]; r != nil {
		user.Roles = r
	}
	tenant, err := s.d.Store.Tenants.GetByID(ctx, p.TenantID)
	if err != nil {
		return nil, err
	}
	codes, _, err := s.d.Store.Permissions.ForUser(ctx, p.TenantID, p.UserID)
	if err != nil {
		return nil, err
	}
	return &Me{User: user, Tenant: tenant, Permissions: codes}, nil
}

// ---------------------------------------------------------------------------

func (s *AuthService) ensureActive(ctx context.Context, tx *repository.Store, tenantID, userID uuid.UUID) error {
	state, err := tx.Users.AuthState(ctx, tenantID, userID)
	if err != nil {
		return mapNotFound(err, "invalid_token", "token is invalid or expired")
	}
	if state.TenantStatus != domain.TenantActive {
		return apperror.Forbidden("tenant_suspended", "tenant is suspended")
	}
	if state.UserStatus != domain.UserActive {
		return apperror.Forbidden("user_inactive", "user account is not active")
	}
	return nil
}

func (s *AuthService) issuePair(ctx context.Context, tx *repository.Store, userID, tenantID uuid.UUID, meta ClientMeta) (*TokenPair, error) {
	return s.issuePairWithID(ctx, tx, uuid.New(), userID, tenantID, meta)
}

func (s *AuthService) issuePairWithID(ctx context.Context, tx *repository.Store, refreshID, userID, tenantID uuid.UUID, meta ClientMeta) (*TokenPair, error) {
	access, accessExp, err := s.d.JWT.Issue(auth.TokenAccess, userID, tenantID, "")
	if err != nil {
		return nil, err
	}
	refresh, refreshExp, err := s.d.JWT.Issue(auth.TokenRefresh, userID, tenantID, refreshID.String())
	if err != nil {
		return nil, err
	}
	if err := tx.Tokens.CreateRefreshSession(ctx, domain.RefreshSession{
		ID: refreshID, TenantID: tenantID, UserID: userID, ExpiresAt: refreshExp,
		UserAgent: truncate(meta.UserAgent, 512), IPAddress: truncate(meta.IP, 64),
	}); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken: access, AccessExpiresAt: accessExp,
		RefreshToken: refresh, RefreshExpiresAt: refreshExp,
		TokenType: "Bearer",
	}, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
