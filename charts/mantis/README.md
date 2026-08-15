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

## There's no published image yet

Build the two images from the repo root, then make them reachable by
whatever cluster you're installing into:

```bash
docker build -f build/Dockerfile.engine -t mantis-engine:dev .
docker build -f build/Dockerfile.web    -t mantis-web:dev    .
```

- **minikube:** `minikube image load mantis-engine:dev && minikube image load mantis-web:dev`,
  then set `pullPolicy: Never` (see below) so it never tries to pull from
  a registry.
- **kind:** `kind load docker-image mantis-engine:dev mantis-web:dev`, same `pullPolicy: Never`.
- **A real cluster:** push both images to a registry it can reach (GHCR,
  ECR, your own), and set `image.engine.repository`/`image.web.repository`
  to the pushed path.

## Install

```bash
helm install mantis ./charts/mantis \
  --namespace mantis --create-namespace \
  --set image.engine.pullPolicy=Never \
  --set image.web.pullPolicy=Never
```

(Drop the two `pullPolicy` overrides once you're pulling from a real
registry instead of a locally-loaded image.)

Then follow the printed NOTES — they tell you exactly how to reach the UI
based on whatever `web.service.type`/`web.ingress.enabled` you chose.

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
| `image.engine.repository` / `image.web.repository` | `mantis-engine` / `mantis-web` | Image names — set to a full registry path once you're pushing images |
| `image.engine.tag` / `image.web.tag` | chart `appVersion` | Image tag |
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
