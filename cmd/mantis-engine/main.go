// Command mantis-engine is the Mantis backend microservice. It reads the Kubernetes
// cluster it runs in (via its ServiceAccount) and serves the resource graph as
// JSON. It holds no UI — the mantis-web service renders the graph and proxies to
// this one.
//
// Configuration (environment variables):
//
//	MANTIS_ENGINE_ADDR   listen address                             (default ":8080")
//	MANTIS_SHOW_ALL      include system-managed ConfigMaps/Secrets  (default "false")
//
// Endpoints: GET /api/graph, GET /healthz (liveness), GET /readyz (readiness).
package main

import (
	"log"

	"github.com/continuumx1/mantis/internal/engine"
	"github.com/continuumx1/mantis/internal/httpx"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
)

func main() {
	addr := httpx.EnvOr("MANTIS_ENGINE_ADDR", ":8080")
	showAll := httpx.EnvOr("MANTIS_SHOW_ALL", "false") == "true"

	client, err := mantiskube.NewClient()
	if err != nil {
		log.Fatalf("mantis-engine: connect to Kubernetes: %v", err)
	}

	server := engine.New(client, showAll)

	log.Printf("mantis-engine: serving graph API on %s", addr)
	log.Printf("mantis-engine: cluster context %q (%s)", client.Context, client.Server)

	if err := httpx.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatalf("mantis-engine: %v", err)
	}
}
