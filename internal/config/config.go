// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Mail     MailConfig
	Platform PlatformConfig
	Onboard  OnboardingConfig
}

type AppConfig struct {
	Name     string
	Env      string // development | staging | production
	LogLevel string
	// PublicBaseURL is the externally reachable base URL of this API. It is
	// used to build the onboarding link sent by email.
	PublicBaseURL string
}

type HTTPConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	TrustedProxies  []string
}

type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
}

type JWTConfig struct {
	Secret           string
	Issuer           string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	PasswordSetupTTL time.Duration
}

type MailConfig struct {
	Driver   string // smtp | log
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type PlatformConfig struct {
	// APIKey protects the platform (super-admin) endpoints used to provision tenants.
	APIKey string
}

type OnboardingConfig struct {
	InviteTTL time.Duration
	// SetPasswordURL is the frontend "set password" page. When set, the verify
	// endpoint redirects the browser there with a password-setup token.
	SetPasswordURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Name:          env("APP_NAME", "user-management"),
			Env:           env("APP_ENV", "development"),
			LogLevel:      env("LOG_LEVEL", "info"),
			PublicBaseURL: strings.TrimRight(env("APP_PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		},
		HTTP: HTTPConfig{
			Port:            env("HTTP_PORT", "8080"),
			ReadTimeout:     envDuration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    envDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     envDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: envDuration("HTTP_SHUTDOWN_TIMEOUT", 20*time.Second),
			TrustedProxies:  envList("HTTP_TRUSTED_PROXIES"),
		},
		Database: DatabaseConfig{
			URL:             env("DATABASE_URL", ""),
			MaxConns:        int32(envInt("DATABASE_MAX_CONNS", 20)),
			MinConns:        int32(envInt("DATABASE_MIN_CONNS", 2)),
			MaxConnLifetime: envDuration("DATABASE_MAX_CONN_LIFETIME", time.Hour),
		},
		JWT: JWTConfig{
			Secret:           env("JWT_SECRET", ""),
			Issuer:           env("JWT_ISSUER", "user-management"),
			AccessTokenTTL:   envDuration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL:  envDuration("JWT_REFRESH_TOKEN_TTL", 7*24*time.Hour),
			PasswordSetupTTL: envDuration("JWT_PASSWORD_SETUP_TTL", 30*time.Minute),
		},
		Mail: MailConfig{
			Driver:   env("MAIL_DRIVER", "smtp"),
			Host:     env("SMTP_HOST", "localhost"),
			Port:     envInt("SMTP_PORT", 1025),
			Username: env("SMTP_USERNAME", ""),
			Password: env("SMTP_PASSWORD", ""),
			From:     env("MAIL_FROM", "no-reply@example.com"),
		},
		Platform: PlatformConfig{
			APIKey: env("PLATFORM_API_KEY", ""),
		},
		Onboard: OnboardingConfig{
			InviteTTL:      envDuration("ONBOARDING_INVITE_TTL", 72*time.Hour),
			SetPasswordURL: env("ONBOARDING_SET_PASSWORD_URL", ""),
		},
	}
	return cfg, cfg.validate()
}

func (c *Config) IsProduction() bool { return c.App.Env == "production" }

func (c *Config) validate() error {
	var errs []error
	if c.Database.URL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.JWT.Secret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 characters"))
	}
	if len(c.Platform.APIKey) < 16 {
		errs = append(errs, errors.New("PLATFORM_API_KEY must be at least 16 characters"))
	}
	if c.Mail.Driver != "smtp" && c.Mail.Driver != "log" {
		errs = append(errs, fmt.Errorf("MAIL_DRIVER must be 'smtp' or 'log', got %q", c.Mail.Driver))
	}
	return errors.Join(errs...)
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envList(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
