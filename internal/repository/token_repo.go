package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
)

type TokenRepo struct{ db DBTX }

// ---- one-time action tokens ------------------------------------------------

func (r *TokenRepo) CreateActionToken(ctx context.Context, tenantID, userID uuid.UUID, purpose domain.ActionTokenPurpose, hash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_action_tokens (tenant_id, user_id, purpose, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, tenantID, userID, purpose, hash, expiresAt)
	return err
}

// GetActionTokenForUpdate looks a token up by hash and locks it.
func (r *TokenRepo) GetActionTokenForUpdate(ctx context.Context, purpose domain.ActionTokenPurpose, hash string) (*domain.ActionToken, error) {
	var t domain.ActionToken
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, purpose, expires_at, used_at
		FROM user_action_tokens WHERE purpose = $1 AND token_hash = $2 FOR UPDATE`, purpose, hash).
		Scan(&t.ID, &t.TenantID, &t.UserID, &t.Purpose, &t.ExpiresAt, &t.UsedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

func (r *TokenRepo) MarkActionTokenUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE user_action_tokens SET used_at = now() WHERE id = $1`, id)
	return err
}

// InvalidateActionTokens expires all outstanding tokens of a purpose for a user.
func (r *TokenRepo) InvalidateActionTokens(ctx context.Context, tenantID, userID uuid.UUID, purpose domain.ActionTokenPurpose) error {
	_, err := r.db.Exec(ctx, `
		UPDATE user_action_tokens SET used_at = now()
		WHERE tenant_id = $1 AND user_id = $2 AND purpose = $3 AND used_at IS NULL`, tenantID, userID, purpose)
	return err
}

// ---- refresh sessions -----------------------------------------------------

func (r *TokenRepo) CreateRefreshSession(ctx context.Context, s domain.RefreshSession) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO refresh_tokens (id, tenant_id, user_id, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6)`, s.ID, s.TenantID, s.UserID, s.ExpiresAt, s.UserAgent, s.IPAddress)
	return err
}

func (r *TokenRepo) GetRefreshSessionForUpdate(ctx context.Context, tenantID, id uuid.UUID) (*domain.RefreshSession, error) {
	var s domain.RefreshSession
	err := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, expires_at, revoked_at, user_agent, ip_address
		FROM refresh_tokens WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, id).
		Scan(&s.ID, &s.TenantID, &s.UserID, &s.ExpiresAt, &s.RevokedAt, &s.UserAgent, &s.IPAddress)
	if err != nil {
		return nil, notFound(err)
	}
	return &s, nil
}

func (r *TokenRepo) RevokeRefreshSession(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2
		WHERE id = $1 AND revoked_at IS NULL`, id, replacedBy)
	return err
}

func (r *TokenRepo) RevokeAllForUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE tenant_id = $1 AND user_id = $2 AND revoked_at IS NULL`, tenantID, userID)
	return err
}
