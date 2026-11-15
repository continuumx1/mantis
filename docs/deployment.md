# Deployment

## Helm chart

The chart deploys the three services — `mantis-engine` (read-only RBAC),
`mantis-web` (UI + sign-in + `/api` proxy) and `mantis-report` (PDF
reports, no Kubernetes access) — plus the ServiceAccount,
ClusterRole/ClusterRoleBinding, Services, and a Secret holding the
administrator account (see [Security](security.md#signing-in)).

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis \
  --version 1.0.0 --namespace mantis --create-namespace
```

Or from a checkout of the repo: `helm install mantis ./charts/mantis --namespace mantis --create-namespace`.

### Versions

| Chart `version` | `appVersion` / image tag | Notes |
|---|---|---|
| `1.0.0` (current) | `1.0.0` | **Vishwakarma** — first stable release ([release notes](releases/v1.0.0.md)): deterministic topology views, application UI, watch-based sync, `mantis-report` PDF reports, configurable sign-in with no default password. Adds the `watch` verb and `events`/metrics reads to RBAC. See the [changelog](https://github.com/continuumx1/mantis/blob/main/CHANGELOG.md). |
| `0.3.0` | `0.1.0-preview.3` | Adds the Playground demo and docs site; UI theme refresh. No RBAC or API changes. |
| `0.2.0` | `0.1.0-preview.2` | 0 known vulnerabilities (Docker Scout) |
| `0.1.0` | `0.1.0-preview.1` | superseded — had unpatched CVEs, fixed in `preview.2` |

Install an older version by number: `--version 0.3.0`, `--version 0.2.0`,
or `--version 0.1.0`. All are published OCI artifacts under
`cx1tech/mantis` on Docker Hub, not just tags in this repo's history.

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

`mantis-engine`'s and `mantis-report`'s Services are always `ClusterIP` —
there's no value to change that. The engine (which holds the Kubernetes read
access) and the report service are never reachable except through
`mantis-web`. Use TLS on the Ingress (`web.ingress.tls`) whenever Mantis is
reachable beyond your own machine; session cookies are then marked
`Secure`.

### Key values

| Key | Default | Meaning |
|---|---|---|
| `image.repository` | `cx1tech/mantis` | Shared repo for all images — split by a `-engine`/`-web`/`-report` tag suffix |
| `image.tag` | chart-pinned | Version tag, before the suffix |
| `auth.adminUsername` | `admin` | Administrator account name |
| `auth.adminPassword` | `""` | Its password (8+ characters); empty generates a random one, kept across upgrades |
| `auth.existingSecret` | `""` | Use your own Secret (keys `username`, and `passwordHash` (bcrypt) or `password`) |
| `report.enabled` | `true` | Deploy `mantis-report`; `false` removes reports |
| `report.replicaCount` | `1` | Report replicas (stateless; scale freely) |
| `report.maxConcurrent` | `2` | Simultaneous reports per replica |
| `engine.showAll` | `false` | Include system-managed ConfigMaps/Secrets in the graph (`MANTIS_SHOW_ALL`) |
| `web.service.type` | `ClusterIP` | `ClusterIP`/`NodePort`/`LoadBalancer` |
| `web.ingress.enabled` | `false` | Create an Ingress for `mantis-web` |
| `rbac.create` | `true` | Create the read-only ClusterRole/ClusterRoleBinding |
| `networkPolicy.enabled` | `false` | Allow only `mantis-web` → `mantis-engine`, `mantis-web` → `mantis-report` and `mantis-report` → `mantis-engine` (requires a NetworkPolicy-enforcing CNI) |

Full reference: [`charts/mantis/values.yaml`](https://github.com/continuumx1/mantis/blob/main/charts/mantis/values.yaml).

## Container images

Each service builds from its own multi-stage, distroless, nonroot
Dockerfile:

```bash
docker build -f build/Dockerfile.engine -t cx1tech/mantis:dev-engine .
docker build -f build/Dockerfile.web    -t cx1tech/mantis:dev-web .
docker build -f build/Dockerfile.report -t cx1tech/mantis:dev-report .
```

Using your own build with the chart:

- **minikube:** `minikube image load cx1tech/mantis:dev-engine` (and `-web`, `-report`), then `--set image.tag=dev --set image.pullPolicy=Never`.
- **kind:** `kind load docker-image cx1tech/mantis:dev-engine cx1tech/mantis:dev-web cx1tech/mantis:dev-report`, with the same `--set` flags.
- **A real cluster:** push to a registry it can reach and `--set image.repository=<your-registry>/mantis --set image.tag=<tag>`.

## Configuration reference

All three services are configured entirely through environment variables.

| Service | Variable | Default | Meaning |
|---|---|---|---|
| `mantis-engine` | `MANTIS_ENGINE_ADDR` | `:8080` | Listen address |
| `mantis-engine` | `MANTIS_SHOW_ALL` | `false` | Include system-managed ConfigMaps/Secrets in the graph |
| `mantis-engine` | `MANTIS_SYNC_MODE` | `watch` | `watch` (list once, then follow changes) or `poll` |
| `mantis-engine` | `MANTIS_SYNC_INTERVAL` | `20s` | Poll mode only: re-list interval |
| `mantis-web` | `MANTIS_WEB_ADDR` | `:8080` | Listen address |
| `mantis-web` | `MANTIS_ENGINE_URL` | `http://mantis-engine:8080` | Engine base URL to proxy `/api` to |
| `mantis-web` | `MANTIS_REPORT_URL` | *(empty)* | Report service URL for `/api/report`; empty disables reports |
| `mantis-web` | `MANTIS_ADMIN_USERNAME` | `admin` | Administrator account name |
| `mantis-web` | `MANTIS_ADMIN_PASSWORD_HASH` | — | bcrypt hash of its password (preferred) |
| `mantis-web` | `MANTIS_ADMIN_PASSWORD` | — | Or the password itself (8+ characters), hashed at startup |
| `mantis-report` | `MANTIS_REPORT_ADDR` | `:8080` | Listen address |
| `mantis-report` | `MANTIS_ENGINE_URL` | `http://mantis-engine:8080` | Engine base URL for snapshots |
| `mantis-report` | `MANTIS_REPORT_MAX_CONCURRENT` | `2` | Simultaneous reports per replica |

`mantis-web` refuses to start without one of the two password variables.

Endpoints:

- `mantis-engine`: `GET /api/graph` (`?since=<revision>` for deltas), `GET /api/graph/search`, `GET /api/graph/neighbors`, `GET /api/resource`, `GET /api/events`, `GET /api/cluster/capacity`, `GET /api/sync/status`, `GET /internal/v1/snapshot` (for `mantis-report`), `GET /healthz`, `GET /readyz`
- `mantis-web`: `GET /` (UI), `GET`/`POST /login`, `POST /logout`, `GET /api/session`, `/api/*` (proxied to the engine), `POST /api/report` (proxied to `mantis-report`), `GET /healthz`
- `mantis-report`: `POST /api/report`, `GET /healthz`, `GET /readyz`
