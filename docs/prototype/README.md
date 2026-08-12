# Visual prototype — cluster resource graph

`cluster-graph.html` is a **design prototype**, not a working feature. Open it in
any browser (it is fully self-contained — no build step, no network):

```bash
open docs/prototype/cluster-graph.html   # macOS
```

## What it shows

An interactive, browser-based rendering of a Kubernetes cluster as a resource
graph — the direction KNW is evolving toward: **complete** (nothing left out by
hand), **visual**, and **interactive**.

- Resources as nodes, coloured by kind; relationships as edges.
- Namespaces as **regions, not boundaries** — edges cross freely (the shared
  `Node` connects all three namespaces).
- Cluster-scoped resources (`Node`, `PersistentVolume`, `StorageClass`) in their
  own zone.
- Click any node for a detail panel: health and container readiness, resource
  requests/limits, Service endpoints, HPA, relationships, and YAML.
- Honest by design: an Ingress backend that does not exist renders as a dashed
  "NOT FOUND" ghost node.
- Drag to arrange, scroll to zoom, drag the background to pan.

## Important

The data in this file is **hand-authored** to match a demo cluster
(`knw-demo` + `default` + `kube-system` on minikube). It is **not** wired to a
live cluster — it exists to validate the visual/interactive direction.

The real version would be `knw serve`: the existing graph engine
(`internal/graph`) already produces exactly this node/edge model, plus health,
container status, resource limits, and endpoints. A future web renderer would
consume that model and draw this against any live cluster. This prototype is the
face the engine has been missing, not a rewrite of it.
