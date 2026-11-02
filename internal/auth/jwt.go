// Package auth contains JWT, password and token primitives.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/config"
)

// TokenType distinguishes the purpose of a JWT. Each type is also issued for
// a different audience, so a token of one type can never be accepted where
// another is expected.
type TokenType string

const (
	TokenAccess        TokenType = "access"
	TokenRefresh       TokenType = "refresh"
	TokenPasswordSetup TokenType = "password_setup"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

type Claims struct {
	TenantID string    `json:"tid"`
	Type     TokenType `json:"typ"`
	jwt.RegisteredClaims
}

func (c *Claims) UserID() (uuid.UUID, error)     { return uuid.Parse(c.Subject) }
func (c *Claims) TenantUUID() (uuid.UUID, error) { return uuid.Parse(c.TenantID) }

type JWTManager struct {
	secret []byte
	issuer string
	ttl    map[TokenType]time.Duration
	now    func() time.Time
}

func NewJWTManager(cfg config.JWTConfig) *JWTManager {
	return &JWTManager{
		secret: []byte(cfg.Secret),
		issuer: cfg.Issuer,
		ttl: map[TokenType]time.Duration{
			TokenAccess:        cfg.AccessTokenTTL,
			TokenRefresh:       cfg.RefreshTokenTTL,
			TokenPasswordSetup: cfg.PasswordSetupTTL,
		},
		now: time.Now,
	}
}

func (m *JWTManager) TTL(t TokenType) time.Duration { return m.ttl[t] }

func audience(t TokenType) string { return "um:" + string(t) }

// Issue signs a token of the given type. jti may be empty, in which case a
// random one is generated.
func (m *JWTManager) Issue(t TokenType, userID, tenantID uuid.UUID, jti string) (string, time.Time, error) {
	ttl, ok := m.ttl[t]
	if !ok {
		return "", time.Time{}, fmt.Errorf("unknown token type %q", t)
	}
	if jti == "" {
		jti = uuid.NewString()
	}
	now := m.now()
	exp := now.Add(ttl)
	claims := Claims{
		TenantID: tenantID.String(),
		Type:     t,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{audience(t)},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti,
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	return signed, exp, err
}

// Parse validates signature, algorithm, issuer, audience, expiry and token type.
func (m *JWTManager) Parse(raw string, expected TokenType) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims,
		func(t *jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(audience(expected)),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	if claims.Type != expected {
		return nil, ErrInvalidToken
	}
	if _, err := claims.UserID(); err != nil {
		return nil, ErrInvalidToken
	}
	if _, err := claims.TenantUUID(); err != nil {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
