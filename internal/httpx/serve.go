// Package httpx holds small HTTP helpers shared by the Mantis service binaries.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ListenAndServe runs an HTTP server on addr until SIGINT/SIGTERM, then drains
// in-flight requests so a rolling update or scale-down terminates cleanly. It is
// the standard entry point for every Mantis service. handler is wrapped in
// Recover so a panic in any request logs and 500s instead of crashing the
// process out from under this function.
func ListenAndServe(addr string, handler http.Handler) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           Recover(handler),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("startup", "event", "listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		slog.Error("startup failed", "event", "listen_failed", "addr", addr, "error", err.Error())
		return fmt.Errorf("listen on %s: %w", addr, err)
	case sig := <-stop:
		slog.Info("shutdown", "event", "shutdown_start", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := server.Shutdown(ctx)
		if err != nil {
			slog.Error("shutdown", "event", "shutdown_error", "error", err.Error())
		} else {
			slog.Info("shutdown", "event", "shutdown_complete")
		}
		return err
	}
}

// EnvOr returns the value of environment variable key, or fallback when unset or
// empty.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// WriteText writes a plain-text response body. It is the standard way Mantis
// services answer health/readiness probes.
func WriteText(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}
