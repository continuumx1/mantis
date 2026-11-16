// Command mantis-engine is the Mantis backend microservice. It reads the Kubernetes
// cluster it runs in (via its ServiceAccount) and serves the resource graph as
// JSON. It holds no UI — the mantis-web service renders the graph and proxies to
// this one.
//
// Configuration (environment variables):
//
//	MANTIS_ENGINE_ADDR     listen address                             (default ":8080")
//	MANTIS_SHOW_ALL        include system-managed ConfigMaps/Secrets  (default "false")
//	MANTIS_SYNC_MODE       "watch" (default: list once, then follow watch streams) or "poll"
//	MANTIS_SYNC_INTERVAL   poll mode only: re-read interval, e.g. "20s" (default "20s")
//	MANTIS_CLUSTER_NAME    name shown for the cluster in the UI and reports
//	                       (default: the kubeconfig context, or "in-cluster")
//
// The engine reads the cluster on its own background schedule (see
// internal/engine/sync.go), not on demand per request — MANTIS_SYNC_INTERVAL
// controls that schedule, independent of how often mantis-web's UI polls
// GET /api/graph, which now just reads whatever the background loop last
// published.
//
// Endpoints: GET /api/graph, GET /api/sync/status, GET /healthz (liveness),
// GET /readyz (readiness).
package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/continuumx1/mantis/internal/engine"
	"github.com/continuumx1/mantis/internal/httpx"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
)

// version identifies this build (e.g. "1.0.0"). It's set at
// image-build time via -ldflags "-X main.version=..." (see
// build/Dockerfile.engine); local `go run`/`go build` with no ldflags
// leaves it at "dev". Logged at startup so `kubectl logs` on a running Pod
// tells you exactly which tag it came from, without depending on the
// Deployment spec still matching what's actually running.
var version = "dev"

func main() {
	httpx.InitLogging("mantis-engine")
	slog.Info("startup", "event", "boot", "version", version)

	addr := httpx.EnvOr("MANTIS_ENGINE_ADDR", ":8080")
	showAll := httpx.EnvOr("MANTIS_SHOW_ALL", "false") == "true"
	syncInterval, err := time.ParseDuration(httpx.EnvOr("MANTIS_SYNC_INTERVAL", "20s"))
	if err != nil {
		slog.Error("startup failed", "event", "bad_sync_interval", "error", err.Error())
		os.Exit(1)
	}

	client, err := mantiskube.NewClient()
	if err != nil {
		slog.Error("startup failed", "event", "kubernetes_connect_failed", "error", err.Error())
		os.Exit(1)
	}

	// Inside a cluster there is no context name ("in-cluster"), so let the
	// operator say which cluster this is: it is shown in the header, the
	// dashboard and on reports.
	if name := strings.TrimSpace(os.Getenv("MANTIS_CLUSTER_NAME")); name != "" {
		client.Context = name
	}

	server := engine.New(client, showAll, syncInterval)
	server.SetSyncMode(httpx.EnvOr("MANTIS_SYNC_MODE", engine.SyncModeWatch))
	server.Start(context.Background())

	slog.Info("startup", "event", "ready_to_serve", "addr", addr, "sync_interval", syncInterval.String(),
		"cluster_context", client.Context, "cluster_server", client.Server, "show_all", showAll)

	if err := httpx.ListenAndServe(addr, server.Handler()); err != nil {
		slog.Error("fatal", "event", "serve_failed", "error", err.Error())
		os.Exit(1)
	}
}
