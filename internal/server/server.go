package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

const shutdownTimeout = 10 * time.Second

// New returns an HTTP server with conservative timeouts.
func New(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// Run serves until ctx is cancelled, then shuts down gracefully.
func Run(ctx context.Context, log *slog.Logger, srv *http.Server) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
