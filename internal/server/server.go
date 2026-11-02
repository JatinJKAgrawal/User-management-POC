package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jatinjkagrawal/user-management-poc/internal/config"
)

// Run serves until ctx is cancelled, then shuts down gracefully.
func Run(ctx context.Context, cfg config.HTTPConfig, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down http server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
