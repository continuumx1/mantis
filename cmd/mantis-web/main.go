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
	"log"

	"github.com/continuumx1/mantis/internal/httpx"
	"github.com/continuumx1/mantis/internal/web"
)

func main() {
	addr := httpx.EnvOr("MANTIS_WEB_ADDR", ":8080")
	engineURL := httpx.EnvOr("MANTIS_ENGINE_URL", "http://mantis-engine:8080")

	handler, err := web.NewHandler(engineURL)
	if err != nil {
		log.Fatalf("mantis-web: %v", err)
	}

	log.Printf("mantis-web: serving UI on %s", addr)
	log.Printf("mantis-web: proxying /api → %s", engineURL)
	log.Printf("mantis-web: ⚠ public-preview auth active (admin/admin, see internal/web/auth.go) — not a production login, replace before any non-preview deployment")

	if err := httpx.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("mantis-web: %v", err)
	}
}
