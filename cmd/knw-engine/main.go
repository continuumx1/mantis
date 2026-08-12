// Command knw-engine is the KNW backend microservice. It reads the Kubernetes
// cluster it runs in (via its ServiceAccount) and serves the resource graph as
// JSON. It holds no UI — the knw-web service renders the graph and proxies to
// this one.
//
// Configuration (environment variables):
//
//	KNW_ENGINE_ADDR   listen address                             (default ":8080")
//	KNW_SHOW_ALL      include system-managed ConfigMaps/Secrets  (default "false")
//
// Endpoints: GET /api/graph, GET /healthz (liveness), GET /readyz (readiness).
package main

import (
	"log"

	"github.com/continuumx1/knw/internal/engine"
	"github.com/continuumx1/knw/internal/httpx"
	knwkube "github.com/continuumx1/knw/internal/kubernetes"
)

func main() {
	addr := httpx.EnvOr("KNW_ENGINE_ADDR", ":8080")
	showAll := httpx.EnvOr("KNW_SHOW_ALL", "false") == "true"

	client, err := knwkube.NewClient()
	if err != nil {
		log.Fatalf("knw-engine: connect to Kubernetes: %v", err)
	}

	server := engine.New(client, showAll)

	log.Printf("knw-engine: serving graph API on %s", addr)
	log.Printf("knw-engine: cluster context %q (%s)", client.Context, client.Server)

	if err := httpx.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatalf("knw-engine: %v", err)
	}
}
