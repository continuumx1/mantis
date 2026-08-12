// Command knw-web is the KNW frontend microservice. It serves the resource-graph
// UI and reverse-proxies every /api/ request to the knw-engine backend, so the
// browser stays same-origin (no CORS) and the engine can remain a private
// ClusterIP. It never talks to the Kubernetes API itself.
//
// Configuration (environment variables):
//
//	KNW_WEB_ADDR     listen address        (default ":8080")
//	KNW_ENGINE_URL   knw-engine base URL   (default "http://knw-engine:8080")
//
// Endpoints: GET / (UI), any /api/* (proxied to the engine), GET /healthz.
package main

import (
	"log"

	"github.com/continuumx1/knw/internal/httpx"
	"github.com/continuumx1/knw/internal/web"
)

func main() {
	addr := httpx.EnvOr("KNW_WEB_ADDR", ":8080")
	engineURL := httpx.EnvOr("KNW_ENGINE_URL", "http://knw-engine:8080")

	handler, err := web.NewHandler(engineURL)
	if err != nil {
		log.Fatalf("knw-web: %v", err)
	}

	log.Printf("knw-web: serving UI on %s", addr)
	log.Printf("knw-web: proxying /api → %s", engineURL)

	if err := httpx.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("knw-web: %v", err)
	}
}
