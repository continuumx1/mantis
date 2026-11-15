# Mantis Helm chart

Deploys the three Mantis services — `mantis-engine` (reads the cluster,
read-only RBAC), `mantis-web` (UI + sign-in + `/api` proxy) and
`mantis-report` (password-protected PDF reports, no Kubernetes access) —
plus the ServiceAccount, ClusterRole/ClusterRoleBinding, Services,
optional network policies, and a Secret holding the administrator account.
See the repo's top-level [README](../../README.md) and the
[docs site](https://continuumx1.github.io/mantis/) for what Mantis is and
how it works; this file is just the chart's own usage notes.

## Versions

The chart's own `version` (Helm's version, in `Chart.yaml`) bumps every
time the chart is pinned to a new image; `image.tag` in `values.yaml` is a
hardcoded literal to match, not computed — each chart version deploys one
exact, known-good image, deliberately, not "whatever's currently latest."

| Chart `version` | `appVersion` / image tag | Git tag | Notes |
|---|---|---|---|
| `1.0.0` (current) | `1.0.0` | `v1.0.0` | First stable release: deterministic topology views, application UI, watch-based sync, the `mantis-report` service, and a configurable administrator account (generated password, no default). RBAC adds the read-only `watch` verb, `events` and metrics `nodes`. See [CHANGELOG.md](../../CHANGELOG.md). |
| `0.3.0` | `0.1.0-preview.3` | `v0.3.0` | Adds the static, no-backend Playground demo and the public docs site; mantis-web's UI moved to a neutral charcoal-grey theme. No RBAC or API changes. |
| `0.2.0` | `0.1.0-preview.2` | `v0.2.0` | 0 known vulnerabilities (Docker Scout) |
| `0.1.0` | `0.1.0-preview.1` | `v0.1.0-preview.1` | superseded — had unpatched CVEs in `golang.org/x/net`/`x/sys`/`x/text`, fixed in `preview.2` |

All versions are published as OCI artifacts — install any of them by
number, no clone needed:

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis --version 1.0.0
# or, to reproduce an earlier release exactly:
helm install mantis oci://registry-1.docker.io/cx1tech/mantis --version 0.3.0
helm install mantis oci://registry-1.docker.io/cx1tech/mantis --version 0.2.0
helm install mantis oci://registry-1.docker.io/cx1tech/mantis --version 0.1.0
```

To instead get an older chart's *source* (e.g. to modify it), the git tags
above check out the exact commit each version was built from:
`git checkout v0.1.0-preview.1 -- charts/mantis`. Overriding just the tag
on the current chart also works for a quick look —
`--set image.tag=0.3.0` — but that runs an older image under 1.0.0's
templates, fine for a quick comparison, not for reproducing that release
exactly.

## Install

The chart itself is published too — no clone required:

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis \
  --version 1.0.0 --namespace mantis --create-namespace
```

Or, from a checkout of this repo:

```bash
helm install mantis ./charts/mantis --namespace mantis --create-namespace
```

Either way, images pull straight from Docker Hub by default — nothing to
build. Then follow the printed NOTES — they tell you how to reach the UI
for the `web.service.type`/`web.ingress.enabled` you chose, and how to read
the generated administrator password.

### Sign-in

There is no default password. Pick one of:

```bash
# 1. Let the chart generate one (default), then read it:
kubectl get secret mantis-mantis-admin -n mantis -o jsonpath='{.data.password}' | base64 -d; echo

# 2. Choose it (8+ characters):
helm upgrade --install mantis ./charts/mantis -n mantis --set auth.adminPassword='<password>'

# 3. Bring your own Secret with keys username + passwordHash (bcrypt) or password:
kubectl create secret generic mantis-login -n mantis \
  --from-literal=username=admin \
  --from-literal=passwordHash="$(htpasswd -nbBC 10 '' '<password>' | tr -d ':\n')"
helm upgrade --install mantis ./charts/mantis -n mantis --set auth.existingSecret=mantis-login
```

A generated password is kept across upgrades. Changing `auth.adminPassword`
restarts `mantis-web`, which signs everyone out.

### Using your own build instead

Building from source (see the repo root's `build/Dockerfile.*`) is one
command per service, if you're testing a change:

```bash
docker build -f build/Dockerfile.engine -t cx1tech/mantis:dev-engine .
docker build -f build/Dockerfile.web    -t cx1tech/mantis:dev-web    .
docker build -f build/Dockerfile.report -t cx1tech/mantis:dev-report .
```

- **minikube:** `minikube image load` each image, then
  `--set image.tag=dev --set image.pullPolicy=Never` so it never tries Docker Hub.
- **kind:** `kind load docker-image cx1tech/mantis:dev-engine cx1tech/mantis:dev-web cx1tech/mantis:dev-report`, with the same `--set` flags.
- **A real cluster:** push to a registry it can reach with the same
  `<tag>-engine` / `<tag>-web` / `<tag>-report` suffixes, and
  `--set image.repository=<your-registry>/mantis --set image.tag=<tag>`.

### Expose it

Nothing is exposed outside the cluster by default (`web.service.type` is
`ClusterIP`). Pick one:

```bash
# Quick local look, any cluster:
kubectl port-forward -n mantis svc/mantis-web 8081:8080
# then open http://localhost:8081

# Or, if you have an Ingress controller:
helm upgrade mantis ./charts/mantis -n mantis \
  --set web.ingress.enabled=true \
  --set web.ingress.hosts[0].host=mantis.example.com

# Or a LoadBalancer, if your cluster provisions one:
helm upgrade mantis ./charts/mantis -n mantis \
  --set web.service.type=LoadBalancer
```

## Key values

| Key | Default | Meaning |
|---|---|---|
| `image.repository` | `cx1tech/mantis` | Shared repo for all images — split by a `-engine`/`-web`/`-report` tag suffix |
| `image.tag` | hardcoded literal (currently `1.0.0`, see Versions above) | Version tag, before the suffix is appended |
| `auth.adminUsername` | `admin` | Administrator account name |
| `auth.adminPassword` | `""` | Its password (8+ characters); empty generates one |
| `auth.existingSecret` | `""` | Use your own Secret instead (see Sign-in) |
| `report.enabled` | `true` | Deploy `mantis-report`; `false` removes reports |
| `report.replicaCount` | `1` | Stateless; scale freely |
| `report.maxConcurrent` | `2` | Simultaneous reports per replica |
| `engine.showAll` | `false` | Include system-managed ConfigMaps/Secrets in the graph (`MANTIS_SHOW_ALL`) |
| `web.service.type` | `ClusterIP` | How `mantis-web` is exposed — `ClusterIP`/`NodePort`/`LoadBalancer` |
| `web.ingress.enabled` | `false` | Create an Ingress for `mantis-web` |
| `rbac.create` | `true` | Create the read-only ClusterRole/ClusterRoleBinding `mantis-engine` needs |
| `networkPolicy.enabled` | `false` | Allow only `mantis-web` → `mantis-engine`, `mantis-web` → `mantis-report` and `mantis-report` → `mantis-engine` (requires a NetworkPolicy-enforcing CNI) |

`mantis-engine`'s and `mantis-report`'s Services are always `ClusterIP` —
there's no value to change that. That's deliberate: the engine (which holds
the Kubernetes read access) and the report service are never reachable
except through `mantis-web`. See
[values.yaml](values.yaml) for the full set of knobs (resources, probes,
node selectors, etc.).

## What RBAC does this actually grant?

Exactly `get`/`list`/`watch` on the resource kinds Mantis's code reads — nothing
else, and no write verb anywhere. `templates/rbac.yaml` documents each
rule's justification against the actual Go source next to it, so you can
audit it without cross-referencing anything else.

## Uninstall

```bash
helm uninstall mantis -n mantis
```

The ClusterRole/ClusterRoleBinding and the admin Secret are chart-owned and
go with it — no Mantis-controlled resources will remain in your cluster.
