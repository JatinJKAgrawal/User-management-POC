package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jatinjkagrawal/user-management-poc/internal/config"
)

func newManager() *JWTManager {
	return NewJWTManager(config.JWTConfig{
		Secret: "test-secret-test-secret-test-secret", Issuer: "test",
		AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour, PasswordSetupTTL: time.Minute,
	})
}

func TestIssueAndParse(t *testing.T) {
	m := newManager()
	uid, tid := uuid.New(), uuid.New()
	tok, _, err := m.Issue(TokenAccess, uid, tid, "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Parse(tok, TokenAccess)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, _ := c.UserID(); got != uid {
		t.Errorf("user id = %v, want %v", got, uid)
	}
	if got, _ := c.TenantUUID(); got != tid {
		t.Errorf("tenant id = %v, want %v", got, tid)
	}
}

func TestTokenTypeConfusionIsRejected(t *testing.T) {
	m := newManager()
	refresh, _, _ := m.Issue(TokenRefresh, uuid.New(), uuid.New(), "")
	setup, _, _ := m.Issue(TokenPasswordSetup, uuid.New(), uuid.New(), "")
	for name, tok := range map[string]string{"refresh": refresh, "setup": setup} {
		if _, err := m.Parse(tok, TokenAccess); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s token accepted as access token: %v", name, err)
		}
	}
}

func TestExpiredToken(t *testing.T) {
	m := newManager()
	m.now = func() time.Time { return time.Now().Add(-2 * time.Minute) }
	tok, _, _ := m.Issue(TokenAccess, uuid.New(), uuid.New(), "")
	m.now = time.Now
	if _, err := m.Parse(tok, TokenAccess); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestWrongSecretOrIssuer(t *testing.T) {
	m := newManager()
	tok, _, _ := m.Issue(TokenAccess, uuid.New(), uuid.New(), "")

	other := newManager()
	other.secret = []byte("another-secret-another-secret-xx")
	if _, err := other.Parse(tok, TokenAccess); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token with wrong secret accepted: %v", err)
	}
	other = newManager()
	other.issuer = "someone-else"
	if _, err := other.Parse(tok, TokenAccess); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token with wrong issuer accepted: %v", err)
	}
}

func TestNoneAlgorithmRejected(t *testing.T) {
	// header {"alg":"none","typ":"JWT"} with a plausible payload and no signature
	tok := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiIxIn0."
	if _, err := newManager().Parse(tok, TokenAccess); err == nil {
		t.Fatal("alg=none token accepted")
	}
}
