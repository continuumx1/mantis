# Mantis Playground

`playground.html` (in the directory above this one) is a fork of the real
`index.html` — same rendering engine, same force layout, same panel/search/YAML
UI — with one difference: instead of fetching `/api/graph` and `/api/resource`
from a live `mantis-engine`, it reads static JSON/YAML files from `data/`
below. There is no server, no auth, and no live cluster behind it, which is
also what the banner across the top of the page says.

It exists so someone can experience the actual Mantis UI — not a mockup of
it — before installing anything.

## Keeping it in sync

Because `playground.html` is a fork, not a build artifact, it does not pick up
`index.html` changes automatically — UI-facing commits to `index.html` (search
behavior, status colors, header, panel/YAML rendering, and so on) need their
equivalent applied here by hand. One deliberate, permanent exception: the
background-sync progress feature (`/api/sync/status` polling, the connecting
overlay's live namespace/resource counts) is not mirrored — it reflects
`mantis-engine`'s real progressive sync loop, which has no equivalent against
a static, already-complete JSON fixture, so there is nothing genuine for it to
show here.

## Why static fixtures, not a live demo cluster

A public page that talks to a real, always-on cluster is a real cluster
exposed to the internet, which contradicts the "don't expose Mantis to the
public internet" guidance the project gives everyone else. Capturing real
graphs once and serving them as static files gets the same authenticity (this
is exactly what `mantis-engine` produces against a real cluster — nothing here
is hand-written) with no live attack surface, no ongoing infrastructure, and
nothing that can go down.

## The three scenarios

| Scenario | Namespaces | Nodes | "Distribution" shown | What it demonstrates |
|---|---|---|---|---|
| Web application | 5 (`shop-frontend`, `checkout`, `catalog`, `ingress-nginx`, `monitoring`) | 5 | kubeadm | The common case: Deployments → ReplicaSets → Pods, Services, an Ingress, ConfigMaps, and an HPA — several teams' apps sharing one small cluster. |
| Stateful database | 5 (`data-platform`, `cache`, `search`, `analytics`, `logging`) | 5 | GKE | Two different StatefulSets with real per-Pod storage (`claims` → `bound-to` through to a PersistentVolume) and two Secrets whose values never reach the UI. |
| Broken relationships | 2 (`payments-api`, `platform`) | 3 | EKS | Three real failure modes Mantis surfaces instead of hiding: an Ingress routing to a Service that no longer exists (the red, dashed "not found" node), a Deployment referencing a missing ConfigMap, and a Service whose selector matches zero Pods — sitting next to an unrelated namespace that's completely healthy. |

The kubeadm/GKE/EKS labeling is cosmetic (see "How the fixtures are built"
below) — it exists so the three scenarios read as Mantis working identically
across different real-world distributions, which is the actual product claim
being demonstrated.

Each scenario's *one* original namespace (`shop-frontend`, `data-platform`,
`payments-api`) is a real capture from `mantis-engine` reading a real cluster
running the manifests in `manifests/` — every node, edge, and manifest there
came from the engine, not a human. The other namespaces and the extra physical
Nodes in each scenario are hand-authored by `expand-fixtures.py` (step 4
below): a single real namespace's worth of relationship data doesn't give a
first-time visitor — especially one who's never worked with more than a
handful of resources — much sense of what a cluster with several teams on it
actually looks like. `expand-fixtures.py` clones the real captured shapes
(same fields, same relation directions, same attribute formats
`internal/graph/enrich.go` would produce) rather than inventing a different
schema, so the added namespaces render identically to genuine engine output;
only their *source* is synthetic.

## How the fixtures are built

1. **Apply a scenario's manifests** to any real cluster (this was done against
   a local minikube):

   ```bash
   kubectl apply -f internal/web/ui/playground/manifests/01-web-app.yaml
   ```

2. **Capture it** with `cmd/playground-capture`, a dev-only tool that runs the
   same `graph`/`engine` code the product ships (never built into the
   `mantis-engine`/`mantis-web` images — see `build/Dockerfile.*`) against
   whichever cluster the current kubeconfig context points at:

   ```bash
   go run ./cmd/playground-capture \
     -namespace shop-frontend \
     -context-label "playground · web application (sample data)" \
     -out internal/web/ui/playground/data/web-app
   ```

   This writes `graph.json` (the exact shape `/api/graph` serves) and one
   `resources/<id>.yaml` manifest per resolved, non-Secret node — Secrets are
   never captured, the same rule the real `/api/resource` handler enforces.
   It also drops any cluster-scoped Node/PersistentVolume left over from a
   different scenario sharing the same dev cluster (see
   `pruneForeignClusterScoped` in `cmd/playground-capture/main.go`).

3. **Re-flavor the identity fields** with `sanitize-fixtures.py`: it renames
   the captured Node from the dev cluster's real name to a distro-flavored one
   per scenario (`k8s-worker-01` / a GKE-style node name), strips
   dev-cluster-specific labels and storage-provisioner annotations, and swaps
   in the matching cloud-flavored equivalents (e.g. a `pd.csi.storage.gke.io`
   CSI volume instead of a local hostPath one). Nothing about *what happened
   in the cluster* is changed — only cosmetic identity fields, and only so all
   three scenarios don't visibly reveal the same one dev cluster they were
   actually captured from.

   ```bash
   python3 internal/web/ui/playground/sanitize-fixtures.py
   ```

4. **Tear the scenario down** before capturing the next one:

   ```bash
   kubectl delete namespace shop-frontend
   ```

   Repeat 1–4 for `02-stateful-db.yaml` (`data-platform`) and
   `03-broken-relationships.yaml` (`payments-api`).

5. **Expand each scenario** with `expand-fixtures.py`: adds the extra
   namespaces/nodes described in the table above directly to `graph.json` and
   `resources/`, on top of whatever the three real captures produced. This is
   the one step that isn't a real capture — see the note above the table for
   why, and read the script itself for exactly what it adds; every helper it
   uses (`add_simple_app`, `add_stateful_app`, `add_node`) is a straight
   template of one of the real captured resources, not an invented shape.

   ```bash
   python3 internal/web/ui/playground/expand-fixtures.py
   ```

## Regenerating

Fixtures only need regenerating if `playground.html` should show something new
(a different scenario, an updated feature). Re-run steps 1–4 end to end for
each scenario, then step 5 once at the end — `sanitize-fixtures.py` and
`expand-fixtures.py` both expect to run against freshly captured data, not
output from a previous run of themselves.

## Serving it locally

`playground.html` uses paths relative to its own directory
(`playground/data/...`), so any static file server rooted at
`internal/web/ui/` works:

```bash
cd internal/web/ui && python3 -m http.server 8099
# open http://127.0.0.1:8099/playground.html
```

## Deploying it

This directory + `playground.html` are the whole artifact — no build step, no
backend. Point `mantis-playground.continuumx1.com` (see `MANTIS_PLAYGROUND_URL`
in the website repo) at a static host serving `internal/web/ui/` with
`playground.html` as the entry point, and flip `MANTIS_PLAYGROUND_LIVE` to
`true` once it's live.
