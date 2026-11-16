// Command mantis-report is the Mantis report service: it turns mantis-engine's
// cluster snapshot into a password-protected (AES-256) PDF — overview, health,
// topology, inventory and findings. It has no Kubernetes client; everything it
// describes comes from mantis-engine's internal snapshot API. It is stateless,
// so it scales by adding replicas.
//
//	MANTIS_REPORT_ADDR            listen address (default ":8080")
//	MANTIS_ENGINE_URL             mantis-engine base URL (default "http://mantis-engine:8080")
//	MANTIS_REPORT_MAX_CONCURRENT  simultaneous reports per replica (default 2)
package main

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/continuumx1/mantis/internal/httpx"
	"github.com/continuumx1/mantis/internal/report"
)

var version = "dev"

func main() {
	httpx.InitLogging("mantis-report")
	slog.Info("startup", "event", "boot", "version", version)
	addr := httpx.EnvOr("MANTIS_REPORT_ADDR", ":8080")
	engineURL := httpx.EnvOr("MANTIS_ENGINE_URL", "http://mantis-engine:8080")
	maxConcurrent, err := strconv.Atoi(httpx.EnvOr("MANTIS_REPORT_MAX_CONCURRENT", "2"))
	if err != nil || maxConcurrent < 1 {
		maxConcurrent = 2
	}
	svc := report.NewService(engineURL, maxConcurrent)
	slog.Info("startup", "event", "ready_to_serve", "addr", addr, "engine_url", engineURL, "max_concurrent", maxConcurrent)
	if err := httpx.ListenAndServe(addr, svc.Handler()); err != nil {
		slog.Error("fatal", "event", "serve_failed", "error", err.Error())
		os.Exit(1)
	}
}
