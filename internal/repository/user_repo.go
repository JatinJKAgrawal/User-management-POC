package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jatinjkagrawal/user-management-poc/internal/domain"
)

type UserRepo struct{ db DBTX }

const userCols = `id, tenant_id, email, first_name, last_name, password_hash, status,
	email_verified_at, password_set_at, last_login_at, invited_by, created_at, updated_at`

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.FirstName, &u.LastName, &u.PasswordHash, &u.Status,
		&u.EmailVerifiedAt, &u.PasswordSetAt, &u.LastLoginAt, &u.InvitedBy, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	u.Roles = []domain.RoleRef{}
	return &u, nil
}

type CreateUserParams struct {
	TenantID  uuid.UUID
	Email     string
	FirstName string
	LastName  string
	InvitedBy *uuid.UUID
}

func (r *UserRepo) Create(ctx context.Context, p CreateUserParams) (*domain.User, error) {
	return scanUser(r.db.QueryRow(ctx, `
		INSERT INTO users (tenant_id, email, first_name, last_name, invited_by, status)
		VALUES ($1, $2, $3, $4, $5, 'invited')
		RETURNING `+userCols,
		p.TenantID, p.Email, p.FirstName, p.LastName, p.InvitedBy))
}

func (r *UserRepo) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.User, error) {
	return scanUser(r.db.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE tenant_id = $1 AND id = $2`, tenantID, id))
}

// GetByIDForUpdate locks the user row for the remainder of the transaction.
func (r *UserRepo) GetByIDForUpdate(ctx context.Context, tenantID, id uuid.UUID) (*domain.User, error) {
	return scanUser(r.db.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, id))
}

func (r *UserRepo) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.User, error) {
	return scanUser(r.db.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE tenant_id = $1 AND email = $2`, tenantID, email))
}

type ListUsersFilter struct {
	Status domain.UserStatus
	Search string
	domain.Page
}

func (r *UserRepo) List(ctx context.Context, tenantID uuid.UUID, f ListUsersFilter) ([]domain.User, int, error) {
	where := []string{"tenant_id = $1"}
	args := []any{tenantID}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+escapeLike(f.Search)+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(email ILIKE $%d OR first_name ILIKE $%d OR last_name ILIKE $%d)", n, n, n))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`SELECT %s FROM users WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		userCols, cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *u)
	}
	return out, total, rows.Err()
}

func (r *UserRepo) UpdateProfile(ctx context.Context, tenantID, id uuid.UUID, firstName, lastName string) (*domain.User, error) {
	return scanUser(r.db.QueryRow(ctx, `
		UPDATE users SET first_name = $3, last_name = $4
		WHERE tenant_id = $1 AND id = $2 RETURNING `+userCols, tenantID, id, firstName, lastName))
}

func (r *UserRepo) UpdateStatus(ctx context.Context, tenantID, id uuid.UUID, status domain.UserStatus) (*domain.User, error) {
	return scanUser(r.db.QueryRow(ctx, `
		UPDATE users SET status = $3
		WHERE tenant_id = $1 AND id = $2 RETURNING `+userCols, tenantID, id, status))
}

func (r *UserRepo) MarkEmailVerified(ctx context.Context, tenantID, id uuid.UUID) error {
	return r.execOne(ctx, `
		UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()), status = 'pending_password'
		WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (r *UserRepo) SetPassword(ctx context.Context, tenantID, id uuid.UUID, hash string) error {
	return r.execOne(ctx, `
		UPDATE users SET password_hash = $3, password_set_at = now(), status = 'active'
		WHERE tenant_id = $1 AND id = $2`, tenantID, id, hash)
}

func (r *UserRepo) TouchLastLogin(ctx context.Context, tenantID, id uuid.UUID) error {
	return r.execOne(ctx, `UPDATE users SET last_login_at = now() WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (r *UserRepo) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	return r.execOne(ctx, `DELETE FROM users WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (r *UserRepo) Count(ctx context.Context, tenantID uuid.UUID) (map[domain.UserStatus]int, error) {
	rows, err := r.db.Query(ctx, `SELECT status, count(*) FROM users WHERE tenant_id = $1 GROUP BY status`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.UserStatus]int{}
	for rows.Next() {
		var s domain.UserStatus
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// AuthState returns the data required to authenticate a request in one query.
type AuthState struct {
	Email        string
	UserStatus   domain.UserStatus
	TenantStatus domain.TenantStatus
}

func (r *UserRepo) AuthState(ctx context.Context, tenantID, id uuid.UUID) (*AuthState, error) {
	var s AuthState
	err := r.db.QueryRow(ctx, `
		SELECT u.email, u.status, t.status
		FROM users u JOIN tenants t ON t.id = u.tenant_id
		WHERE u.tenant_id = $1 AND u.id = $2`, tenantID, id).Scan(&s.Email, &s.UserStatus, &s.TenantStatus)
	if err != nil {
		return nil, notFound(err)
	}
	return &s, nil
}

func (r *UserRepo) execOne(ctx context.Context, sql string, args ...any) error {
	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
