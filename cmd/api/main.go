// Command api runs the multi-tenant user management HTTP API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jatinjkagrawal/user-management-poc/internal/auth"
	"github.com/jatinjkagrawal/user-management-poc/internal/config"
	"github.com/jatinjkagrawal/user-management-poc/internal/platform/database"
	"github.com/jatinjkagrawal/user-management-poc/internal/platform/logger"
	"github.com/jatinjkagrawal/user-management-poc/internal/platform/mailer"
	"github.com/jatinjkagrawal/user-management-poc/internal/repository"
	"github.com/jatinjkagrawal/user-management-poc/internal/server"
	"github.com/jatinjkagrawal/user-management-poc/internal/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.App.LogLevel, cfg.IsProduction() || os.Getenv("LOG_FORMAT") == "json")
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := repository.NewStore(pool)
	svc := service.New(service.Deps{
		Store:  store,
		JWT:    auth.NewJWTManager(cfg.JWT),
		Mailer: mailer.New(cfg.Mail, log),
		Config: cfg,
		Log:    log,
	})

	router := server.NewRouter(cfg, log, store, svc)
	return server.Run(ctx, cfg.HTTP, router, log)
}
