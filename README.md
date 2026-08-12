<p align="center">
  <img src="docs/images/knw-mantis-logo.png" alt="KNW mantis mascot" width="420">
</p>
<p align="center">
  <img src="docs/images/knw-logo-text.png" alt="KNW" width="220">
</p>

<h1 align="center">KNW — Know the story of your kubernetes resources</h1>

<p align="center"><em>Every K8s Resource Has a Story.</em></p>

KNW is an open-source Kubernetes context and investigation engine with an
interactive web UI. It discovers the relationships between Kubernetes resources
and draws them as a live graph, so you can see not just *what* exists in a
cluster, but the *context* around it.

> **Status: experimental (v0.1).** KNW is early and under active development.
> It is strictly read-only and safe to run against any cluster, but its
> behaviour and interfaces are still evolving.

Built and maintained by [ContinuumX1 Technologies](https://continuumx1.com).

---

## Why KNW?

Kubernetes tells you *what* exists. A `Pod` has an `ownerReferences` field, a
`Service` has a `selector`, an `Ingress` has a `backend` — but you have to hold
all of that in your head and stitch it together yourself to answer everyday
questions:

- Why does this pod exist, and what created it?
- Which pods does this service actually route to?
- This pod is stuck `Pending` — what is it waiting on?
- This ingress returns 503 — does the service it points at even exist?

KNW turns the raw metadata into an explained, linked graph: every resource is a
node, every relationship is a typed edge, and namespaces are drawn as regions.
References that point at something which does not exist are flagged, not hidden.

## What makes it different

KNW is not trying to replace the tools you already use — it solves a different
problem.

| Tool | Its job | KNW's job |
|------|---------|-----------|
| `kubectl` | Query and mutate resources | Explain how resources relate |
| k9s | Interactive cluster navigation | Context and investigation |
| Prometheus / Grafana | Metrics and dashboards | Relationships, not metrics |
| ArgoCD | "Does live match Git?" (GitOps sync) | "What's the story around this resource?" |

Unlike a GitOps or dashboard tool, KNW works on **any** resource whether or not
it was deployed through a pipeline, is strictly **read-only** — it never mutates
your cluster — and runs on **any** Kubernetes distribution (minikube, kind,
kubeadm, RKE2, EKS, GKE, AKS, …) because it talks to the standard Kubernetes API,
never to a distro.

## Architecture

KNW runs as **two independently deployable services** on top of a shared,
interface-agnostic relationship engine.

```
                Browser
                   │  http (same-origin)
                   ▼
            ┌──────────────┐   /api proxy   ┌──────────────┐   read-only   ┌──────────────┐
            │   knw-web    │ ─────────────► │  knw-engine  │ ────────────► │ Kubernetes   │
            │  (frontend)  │                │  (backend)   │  client-go    │     API      │
            │  UI + proxy  │ ◄───────────── │  graph JSON  │ ◄──────────── │              │
            └──────────────┘                └──────────────┘               └──────────────┘
              public / Ingress                private ClusterIP
```

- **`knw-engine` (backend)** reads the cluster and serves the resource graph as
  JSON. It holds the read-only credentials and never renders UI. Inside a cluster
  it authenticates with its Pod ServiceAccount; locally it uses your kubeconfig —
  the same binary, no code change.
- **`knw-web` (frontend)** serves the graph UI and reverse-proxies `/api` to
  `knw-engine`. The browser only ever talks to `knw-web`, so there is no CORS and
  the engine can stay a private `ClusterIP` reachable only from the frontend.

Both services are projections of the same `graph.Context`.

```
cmd/
  knw-engine/         backend service entry point (main.go)
  knw-web/            frontend service entry point (main.go)
internal/
  graph/              relationship model — ResourceRef, typed Relation,
                      resolvers, Context, and the whole-cluster graph builder
  engine/             backend: graph JSON projection (dto.go) + HTTP handlers
                      (server.go): /api/graph, /healthz, /readyz
  web/                frontend: embedded UI + /api reverse-proxy
    ui/               the interactive graph (single-page, no external assets)
  kubernetes/         read-only client (in-cluster ServiceAccount OR kubeconfig)
  httpx/              shared HTTP serving (graceful shutdown, env config)
build/
  Dockerfile.engine   backend image (distroless, nonroot)
  Dockerfile.web      frontend image (distroless, nonroot)
```

## Running locally

Requires **Go 1.26+** and a reachable cluster (any distribution).

```bash
git clone https://github.com/continuumx1/knw.git
cd knw

# Backend (engine) — reads the cluster via your kubeconfig
KNW_ENGINE_ADDR=":8080" go run ./cmd/knw-engine

# Frontend (web) — in a second terminal; serves the UI and proxies /api to the engine
KNW_WEB_ADDR=":8081" KNW_ENGINE_URL="http://127.0.0.1:8080" go run ./cmd/knw-web
```

Open <http://127.0.0.1:8081> and you get the live graph of your cluster. **Click**
a resource for its details and relationships, **drag** to arrange, **scroll** to
zoom, **Refresh** to re-read the cluster.

## Configuration

Both services are configured entirely through environment variables — exactly
what a Helm chart will template.

| Service | Env var | Default | Meaning |
|---------|---------|---------|---------|
| `knw-engine` | `KNW_ENGINE_ADDR` | `:8080` | listen address |
| `knw-engine` | `KNW_SHOW_ALL` | `false` | include system-managed ConfigMaps/Secrets |
| `knw-web` | `KNW_WEB_ADDR` | `:8080` | listen address |
| `knw-web` | `KNW_ENGINE_URL` | `http://knw-engine:8080` | engine base URL to proxy `/api` to |

Endpoints:

- `knw-engine`: `GET /api/graph`, `GET /healthz` (liveness), `GET /readyz` (readiness)
- `knw-web`: `GET /` (UI), `GET /api/*` (proxied to the engine), `GET /healthz`

## Container images

One image per service, multi-stage, distroless, nonroot:

```bash
docker build -f build/Dockerfile.engine -t knw-engine:dev .
docker build -f build/Dockerfile.web    -t knw-web:dev .
```

Inside a cluster, `knw-engine` uses its Pod ServiceAccount, which needs read-only
(`get`/`list`) access to the resource kinds KNW maps. Helm packaging of the two
Deployments, Services, and RBAC is the next step.

## Relationship model

Relationships are named for their **Kubernetes semantics**, not for convenience.

| Relationship | Meaning | Source |
|--------------|---------|--------|
| `controlled-by` | A resource is controlled by its owner | `ownerReferences` |
| `runs-on` | A Pod is scheduled onto a Node | `pod.spec.nodeName` |
| `selects` | A Service selects Pods | `service.spec.selector` |
| `serves` | A Service actually backs a Pod | EndpointSlices |
| `routes-to` | An Ingress routes to a Service | ingress backends |
| `references` | A Pod consumes config via the environment | `env` / `envFrom` |
| `mounts` | A Pod mounts config as a volume | `volumes` |
| `claims` | A Pod claims a PersistentVolumeClaim | `volumes[].persistentVolumeClaim` |
| `bound-to` | A PVC is bound to a PersistentVolume | `pvc.spec.volumeName` |

A key principle: KNW distinguishes **facts** it read from the API from
references it could not verify. A target that was checked and found missing is
shown as a "not found" node; one that exists is shown plainly. KNW never guesses.

## Current limitations

- **Whole-cluster snapshot per load.** The graph reflects the cluster at the time
  you load or refresh it; there is no live watch/stream yet.
- **Compact detail.** The detail panel shows the attributes the engine computes
  today (health, storage class, endpoints, …). Richer per-resource detail is
  planned.
- **Built-in kinds only.** Custom resources (CRDs) are not mapped yet.
- **No change / history / GitOps awareness** — KNW explains the current state,
  not what changed or why.

## Roadmap

**Current**

- Relationship engine with a structured `Context`
- Whole-cluster graph as a two-service web application
- Any-distribution support via standard kubeconfig / in-cluster auth
- Verified dangling-reference detection

**Planned**

- Helm chart (two Deployments, two Services, read-only RBAC)
- Richer per-resource detail in the UI
- Namespace / kind filtering and search
- Live updates (watch) instead of snapshot-on-refresh

**Future**

- Change detection and correlation (Git, GitOps, Helm)
- Custom resource (CRD) support

Priorities evolve from real use; nothing above is a commitment.

## Development

```bash
go build ./...     # build both services
go test ./...      # run tests
go vet ./...       # static checks
gofmt -l .         # formatting (should print nothing)
```

Relationship resolution is the core logic and is covered by unit tests using a
fake Kubernetes client, so most behaviour can be verified without a live
cluster.

## Contributing

Contributions are welcome. Please keep changes small and focused, follow
[Conventional Commits](https://www.conventionalcommits.org/), and include tests
for new relationship logic. Contribution and security-reporting guidelines will
be added as the project grows.


## License

KNW is licensed under the [Apache License 2.0](LICENSE).

Copyright 2026 ContinuumX1 Technologies Private Limited.

The KNW name, logo, and mascot are trademarks of ContinuumX1 Technologies and
are not covered by the software license. See [NOTICE](NOTICE).
