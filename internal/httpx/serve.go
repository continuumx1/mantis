// Package httpx holds small HTTP helpers shared by the KNW service binaries.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ListenAndServe runs an HTTP server on addr until SIGINT/SIGTERM, then drains
// in-flight requests so a rolling update or scale-down terminates cleanly. It is
// the standard entry point for every KNW service.
func ListenAndServe(addr string, handler http.Handler) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("listen on %s: %w", addr, err)
	case <-stop:
		log.Print("shutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
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

// WriteText writes a plain-text response body. It is the standard way KNW
// services answer health/readiness probes.
func WriteText(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}
