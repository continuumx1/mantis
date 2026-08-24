# Mantis Helm chart

Deploys both Mantis services — `mantis-engine` (reads the cluster,
read-only RBAC) and `mantis-web` (UI + login + `/api` proxy) — plus the
ServiceAccount, ClusterRole/ClusterRoleBinding, and Services they need.
See the repo's top-level [README](../../README.md) and
[docs/USER_GUIDE.md](../../docs/USER_GUIDE.md) for what Mantis is and how
it works; this file is just the chart's own usage notes.

> **Public Preview.** This chart deploys the current login gate as-is: a
> single hardcoded `admin`/`admin` credential, not real authentication.
> Fine for your own machine or team; don't expose it further without
> reading `docs/USER_GUIDE.md`'s "Public Preview notes" first.

## Versions

The chart's own `version` (Helm's version, in `Chart.yaml`) bumps every
time the chart is pinned to a new image; `image.tag` in `values.yaml` is a
hardcoded literal to match, not computed — each chart version deploys one
exact, known-good image, deliberately, not "whatever's currently latest."

| Chart `version` | `appVersion` / image tag | Git tag | Notes |
|---|---|---|---|
| `0.3.0` (current) | `0.1.0-preview.3` | `v0.3.0` | Adds the static, no-backend Playground demo and the public docs site; mantis-web's UI moved to a neutral charcoal-grey theme. No RBAC or API changes. |
| `0.2.0` | `0.1.0-preview.2` | `v0.2.0` | 0 known vulnerabilities (Docker Scout) |
| `0.1.0` | `0.1.0-preview.1` | `v0.1.0-preview.1` | superseded — had unpatched CVEs in `golang.org/x/net`/`x/sys`/`x/text`, fixed in `preview.2` |

All three versions are published as OCI artifacts — install any of them by
number, no clone needed:

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis --version 0.3.0
# or, to reproduce an earlier release exactly:
helm install mantis oci://registry-1.docker.io/cx1tech/mantis --version 0.2.0
helm install mantis oci://registry-1.docker.io/cx1tech/mantis --version 0.1.0
```

To instead get an older chart's *source* (e.g. to modify it), the git tags
above check out the exact commit each version was built from:
`git checkout v0.1.0-preview.1 -- charts/mantis`. Overriding just the tag
on the current chart also works for a quick look —
`--set image.tag=0.1.0-preview.2` — but that runs preview.2's image under
preview.3's templates, fine for a quick comparison, not for reproducing
that release exactly.

## Install

The chart itself is published too — no clone required:

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis \
  --version 0.3.0 --namespace mantis --create-namespace
```

Or, from a checkout of this repo:

```bash
helm install mantis ./charts/mantis --namespace mantis --create-namespace
```

Either way, images pull straight from Docker Hub by default — nothing to
build. Then follow the printed NOTES — they tell you exactly how to reach
the UI based on whatever `web.service.type`/`web.ingress.enabled` you
chose.

### Using your own build instead

Building from source (see the repo root's `build/Dockerfile.engine` /
`Dockerfile.web`) is still one command away, if you're testing a change:

```bash
docker build -f build/Dockerfile.engine -t mantis-engine:dev .
docker build -f build/Dockerfile.web    -t mantis-web:dev    .
```

- **minikube:** `minikube image load mantis-engine:dev && minikube image load mantis-web:dev`,
  then `--set image.pullPolicy=Never` so it never tries Docker Hub instead.
- **kind:** `kind load docker-image mantis-engine:dev mantis-web:dev`, same `--set image.pullPolicy=Never`.
- **A real cluster:** push to a registry it can reach and
  `--set image.repository=<your-registry>/mantis-yourbuild` (note: your
  build then needs its own `-engine`/`-web` tag suffixes too, matching how
  `image.tag` is templated — see `values.yaml`'s comment — or override
  `image.engine.repository`-style if you'd rather fork the chart's image
  templating to two separate repos).

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
| `image.repository` | `cx1tech/mantis` | Shared repo for both images — see `values.yaml`'s comment for the `-engine`/`-web` tag-suffix scheme |
| `image.tag` | hardcoded literal (currently `0.1.0-preview.3`, see Versions above) | Version tag, before the `-engine`/`-web` suffix is appended |
| `engine.showAll` | `false` | Include system-managed ConfigMaps/Secrets in the graph (`MANTIS_SHOW_ALL`) |
| `web.service.type` | `ClusterIP` | How `mantis-web` is exposed — `ClusterIP`/`NodePort`/`LoadBalancer` |
| `web.ingress.enabled` | `false` | Create an Ingress for `mantis-web` |
| `rbac.create` | `true` | Create the read-only ClusterRole/ClusterRoleBinding `mantis-engine` needs |
| `networkPolicy.enabled` | `false` | Restrict `mantis-engine` ingress to only `mantis-web`'s pods (requires a NetworkPolicy-enforcing CNI) |

`mantis-engine`'s Service is always `ClusterIP` — there's no value to
change that. That's deliberate: the entire point of the two-service split
is that the engine (which holds the Kubernetes read access) is never
reachable except through the frontend's proxy. See
[values.yaml](values.yaml) for the full set of knobs (resources, probes,
node selectors, etc.).

## What RBAC does this actually grant?

Exactly `get`/`list` on the resource kinds Mantis's code reads — nothing
else, and no write verb anywhere. `templates/rbac.yaml` documents each
rule's justification against the actual Go source next to it, so you can
audit it without cross-referencing anything else.

## Uninstall

```bash
helm uninstall mantis -n mantis
```

The ClusterRole/ClusterRoleBinding are chart-owned and go with it — no
Mantis-controlled resources will remain in your cluster.
