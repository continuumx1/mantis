// Command mantis-engine is the Mantis backend microservice. It reads the Kubernetes
// cluster it runs in (via its ServiceAccount) and serves the resource graph as
// JSON. It holds no UI — the mantis-web service renders the graph and proxies to
// this one.
//
// Configuration (environment variables):
//
//	MANTIS_ENGINE_ADDR     listen address                             (default ":8080")
//	MANTIS_SHOW_ALL        include system-managed ConfigMaps/Secrets  (default "false")
//	MANTIS_SYNC_INTERVAL   background cluster-read interval, e.g. "20s" (default "20s")
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
	"time"

	"github.com/continuumx1/mantis/internal/engine"
	"github.com/continuumx1/mantis/internal/httpx"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
)

// version identifies this build (e.g. "0.1.0-preview.1"). It's set at
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

	server := engine.New(client, showAll, syncInterval)
	server.Start(context.Background())

	slog.Info("startup", "event", "ready_to_serve", "addr", addr, "sync_interval", syncInterval.String(),
		"cluster_context", client.Context, "cluster_server", client.Server, "show_all", showAll)

	if err := httpx.ListenAndServe(addr, server.Handler()); err != nil {
		slog.Error("fatal", "event", "serve_failed", "error", err.Error())
		os.Exit(1)
	}
}
