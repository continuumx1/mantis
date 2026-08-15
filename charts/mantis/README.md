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

Images tag as `0.1.0-preview.1`, `0.1.0-preview.2`, … through preview
iterations, then `0.1.0-rc.1` for a release candidate, then `0.1.0` for
stable — this chart's `appVersion` tracks whichever is current. Check
[Docker Hub](https://hub.docker.com/r/cx1tech/mantis/tags) for exactly
what's published if you want to pin an older or newer tag explicitly via
`--set image.tag=...`.

## Install

Published images pull straight from Docker Hub by default — nothing to
build:

```bash
helm install mantis ./charts/mantis --namespace mantis --create-namespace
```

Then follow the printed NOTES — they tell you exactly how to reach the UI
based on whatever `web.service.type`/`web.ingress.enabled` you chose.

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
| `image.tag` | chart `appVersion` (currently `0.1.0-preview.2`) | Version tag, before the `-engine`/`-web` suffix is appended |
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
