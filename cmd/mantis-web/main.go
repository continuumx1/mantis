// Command mantis-web is the Mantis frontend microservice. It serves the resource-graph
// UI and reverse-proxies every /api/ request to the mantis-engine backend, so the
// browser stays same-origin (no CORS) and the engine can remain a private
// ClusterIP. It never talks to the Kubernetes API itself.
//
// Configuration (environment variables):
//
//	MANTIS_WEB_ADDR     listen address        (default ":8080")
//	MANTIS_ENGINE_URL   mantis-engine base URL   (default "http://mantis-engine:8080")
//
// Endpoints: GET / (UI), any /api/* (proxied to the engine), GET /healthz.
package main

import (
	"log/slog"
	"os"

	"github.com/continuumx1/mantis/internal/httpx"
	"github.com/continuumx1/mantis/internal/web"
)

// version identifies this build (e.g. "1.0.0") — see
// cmd/mantis-engine/main.go's version comment; the same build-time
// mechanism applies here.
var version = "dev"

func main() {
	httpx.InitLogging("mantis-web")
	slog.Info("startup", "event", "boot", "version", version)

	addr := httpx.EnvOr("MANTIS_WEB_ADDR", ":8080")
	engineURL := httpx.EnvOr("MANTIS_ENGINE_URL", "http://mantis-engine:8080")

	// Empty disables reports (the UI then explains the service is not deployed).
	reportURL := httpx.EnvOr("MANTIS_REPORT_URL", "")
	admin, err := web.CredentialsFromEnv()
	if err != nil {
		slog.Error("startup failed", "event", "auth_config_invalid", "error", err.Error())
		os.Exit(1)
	}
	handler, err := web.NewHandler(web.Config{EngineURL: engineURL, ReportURL: reportURL, Admin: admin})
	if err != nil {
		slog.Error("startup failed", "event", "handler_init_failed", "error", err.Error())
		os.Exit(1)
	}

	slog.Info("startup", "event", "ready_to_serve", "addr", addr, "engine_url", engineURL,
		"reports", reportURL != "", "admin_user", admin.Username)

	if err := httpx.ListenAndServe(addr, handler); err != nil {
		slog.Error("fatal", "event", "serve_failed", "error", err.Error())
		os.Exit(1)
	}
}
