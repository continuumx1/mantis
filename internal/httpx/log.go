package httpx

import (
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
)

// InitLogging installs a structured (JSON), timestamped logger as the process
// default, tagged with which Mantis service is running. Every slog.Info/Warn/
// Error call anywhere in the process — including inside internal/graph and
// internal/kubernetes, which take no logger of their own — goes through this
// one sink, so a Pod's `kubectl logs` is one consistent, machine-parseable
// stream instead of a mix of plain log.Printf lines and ad-hoc formats.
//
// JSON (not slog's human-readable text handler) is deliberate: these logs are
// meant to be read after the fact, often from a crashed Pod's last lines
// (see Recover below), by whatever log aggregation the cluster already has —
// structured fields survive that trip; formatted prose does not.
func InitLogging(service string) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service)
	slog.SetDefault(logger)
}

// Recover wraps a handler so a panic in any request — a nil pointer from an
// unexpected API response shape, a bad index, anything — is logged with its
// stack trace and turned into a 500, instead of taking down the whole
// process. Without this, one bad request in one goroutine crashes the Pod:
// Kubernetes then restarts it, but the crash itself is invisible except as a
// CrashLoopBackOff with no record of what request triggered it.
//
// This is the last line of defense, not a substitute for handling expected
// failures explicitly (a Kubernetes API error, a bad request body) with their
// own error returns — those should never reach a panic in the first place.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"event", "panic",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
