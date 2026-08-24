# Deployment

## Helm chart

The chart deploys both services — `mantis-engine` (read-only RBAC) and
`mantis-web` (UI + login + `/api` proxy) — plus the ServiceAccount,
ClusterRole/ClusterRoleBinding, and Services they need.

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis \
  --version 0.3.0 --namespace mantis --create-namespace
```

Or from a checkout of the repo: `helm install mantis ./charts/mantis --namespace mantis --create-namespace`.

### Versions

| Chart `version` | `appVersion` / image tag | Notes |
|---|---|---|
| `0.3.0` (current) | `0.1.0-preview.3` | Adds the Playground demo and docs site; UI theme refresh. No RBAC or API changes. |
| `0.2.0` | `0.1.0-preview.2` | 0 known vulnerabilities (Docker Scout) |
| `0.1.0` | `0.1.0-preview.1` | superseded — had unpatched CVEs, fixed in `preview.2` |

Install an older version by number: `--version 0.2.0` or `--version 0.1.0`.
All are published OCI artifacts under `cx1tech/mantis` on Docker Hub, not
just tags in this repo's history.

### Exposing it

Nothing is exposed outside the cluster by default (`mantis-web`'s Service
is `ClusterIP`). Pick one:

```bash
# Quick local look, any cluster:
kubectl port-forward -n mantis svc/mantis-web 8081:8080

# Or, with an Ingress controller:
helm upgrade mantis oci://registry-1.docker.io/cx1tech/mantis -n mantis \
  --set web.ingress.enabled=true \
  --set web.ingress.hosts[0].host=mantis.example.com

# Or a LoadBalancer, if your cluster provisions one:
helm upgrade mantis oci://registry-1.docker.io/cx1tech/mantis -n mantis \
  --set web.service.type=LoadBalancer
```

`mantis-engine`'s Service is always `ClusterIP` — there's no value to
change that. The entire point of the two-service split is that the engine
(which holds the Kubernetes read access) is never reachable except through
the frontend's proxy.

### Key values

| Key | Default | Meaning |
|---|---|---|
| `image.repository` | `cx1tech/mantis` | Shared repo for both images — split by a `-engine`/`-web` tag suffix |
| `image.tag` | chart-pinned | Version tag, before the `-engine`/`-web` suffix |
| `engine.showAll` | `false` | Include system-managed ConfigMaps/Secrets in the graph (`MANTIS_SHOW_ALL`) |
| `web.service.type` | `ClusterIP` | `ClusterIP`/`NodePort`/`LoadBalancer` |
| `web.ingress.enabled` | `false` | Create an Ingress for `mantis-web` |
| `rbac.create` | `true` | Create the read-only ClusterRole/ClusterRoleBinding |
| `networkPolicy.enabled` | `false` | Restrict `mantis-engine` ingress to only `mantis-web`'s pods (requires a NetworkPolicy-enforcing CNI) |

Full reference: [`charts/mantis/values.yaml`](https://github.com/continuumx1/mantis/blob/main/charts/mantis/values.yaml).

## Container images

Each service builds from its own multi-stage, distroless, nonroot
Dockerfile:

```bash
docker build -f build/Dockerfile.engine -t mantis-engine:dev .
docker build -f build/Dockerfile.web    -t mantis-web:dev .
```

Using your own build with the chart:

- **minikube:** `minikube image load mantis-engine:dev && minikube image load mantis-web:dev`, then `--set image.pullPolicy=Never`.
- **kind:** `kind load docker-image mantis-engine:dev mantis-web:dev`, same `--set image.pullPolicy=Never`.
- **A real cluster:** push to a registry it can reach and `--set image.repository=<your-registry>/mantis-yourbuild`.

## Configuration reference

Both services are configured entirely through environment variables.

| Service | Variable | Default | Meaning |
|---|---|---|---|
| `mantis-engine` | `MANTIS_ENGINE_ADDR` | `:8080` | Listen address |
| `mantis-engine` | `MANTIS_SHOW_ALL` | `false` | Include system-managed ConfigMaps/Secrets in the graph |
| `mantis-web` | `MANTIS_WEB_ADDR` | `:8080` | Listen address |
| `mantis-web` | `MANTIS_ENGINE_URL` | `http://mantis-engine:8080` | Base URL of the engine to proxy `/api` to |

Endpoints:

- `mantis-engine`: `GET /api/graph`, `GET /api/resource`, `GET /healthz` (liveness), `GET /readyz` (readiness)
- `mantis-web`: `GET /` (UI), `GET /login`, `GET /api/*` (proxied), `GET /healthz`
