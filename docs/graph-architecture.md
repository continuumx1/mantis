# The graph: model, projections and layout

This page is for contributors. It explains how Mantis turns a cluster into the
picture in the UI, and why it is built the way it is. The short version:

> **One canonical graph. Several projections of it. A deterministic layout.**
> The backend decides what is related to what; the UI only decides how to group,
> fold and position it — and never moves a card by itself.

## 1. The canonical graph (backend)

`mantis-engine` reads the cluster with list calls only (see
`charts/mantis/templates/rbac.yaml` — no write verb exists) and builds one
graph: a node per Kubernetes resource, an edge per relationship.

**Identity.** A resource's graph ID is `<namespace>/<Kind>/<name>`
(`/<Kind>/<name>` when cluster-scoped). Every reference in a Kubernetes spec
points by name, a dangling reference has no UID to point by, and a resource
deleted and recreated under the same name should keep its place on the map —
so the name-based ID is the identity. The resource's `uid`, `apiVersion`,
`labels`, `created` and `rv` (resourceVersion) are carried as fields, so a
recreate is visible as a UID change. Annotations are never copied: they can
hold `kubectl.kubernetes.io/last-applied-configuration`, which for a Secret
contains its data.

**Typed relationships.** Every edge carries:

| field      | meaning |
|------------|---------|
| `type`     | the relation (`controlled-by`, `selects`, `mounts`, …) |
| `category` | `ownership`, `networking`, `configuration`, `storage`, `scheduling`, `security`, `autoscaling` |
| `basis`    | the kind of evidence: `ownership` (ownerReference), `reference` (a name in a spec), `selector` (a label-selector match Mantis computed), `endpoint` (an EndpointSlice record), `inferred` |
| `evidence` | the API field it was read from, e.g. `spec.volumes` |

Mantis reads the Kubernetes API only. It has **no network telemetry**, so no
edge is ever "observed traffic"; the strongest runtime evidence is the
control plane's EndpointSlice record (`serves`), and the UI says so.

| category | relations (source → target) |
|---|---|
| ownership | `controlled-by` (any object → its owner, from `ownerReferences`) |
| scheduling | `runs-on` (Pod → Node) |
| networking | `routes-to` (Ingress → Service), `selects` (Service → Pod, selector), `serves` (Service → Pod, EndpointSlices), `applies-to` (NetworkPolicy → Pod, podSelector) |
| configuration | `references` / `mounts` (Pod → ConfigMap/Secret), `uses-service-account` (Pod → ServiceAccount) |
| storage | `claims` (Pod → PVC), `bound-to` (PVC → PV), `storage-class` (PVC/PV → StorageClass) |
| security | `binds-role` ((Cluster)RoleBinding → (Cluster)Role), `grants-to` (binding → ServiceAccount subject) |
| autoscaling | `scales` (HPA/VPA → workload) |

Selector relations are only computed within the selector's own namespace —
a Service or NetworkPolicy never selects Pods elsewhere.

**Missing vs. unverified.** A reference to something that was listed and is
not there is *not found* (red, dashed). A reference to something Mantis could
not check — its kind is not one Mantis lists (a CRD owner such as an Argo
Rollout), or RBAC forbade listing it — is *unverified* (grey, dashed). Mantis
never claims something is missing when it simply could not look.

**Noise.** The `default` ServiceAccount, `system:` RBAC objects,
ClusterRoleBindings with no ServiceAccount subject, ClusterRoles no visible
binding uses, the root-CA ConfigMap and service-account/Helm Secrets are kept
in the graph (so references to them resolve) but hidden from the payload unless
`MANTIS_SHOW_ALL=true`.

### Keeping the graph current: watch, not poll

By default (`MANTIS_SYNC_MODE=watch`) the engine uses the Kubernetes
list/watch protocol through client-go informers:

1. At startup, one `LIMIT=1` list per kind checks what this identity may read
   (kinds it may not are reported as `skipped`, exactly as before).
2. One list per permitted kind fills an in-memory cache, then a watch per kind
   streams every change. Lost or expired watches are re-established by
   client-go, with resourceVersion handling built in.
3. The graph is rebuilt **from the cache** — no API calls — when changes
   arrive, coalesced (750 ms debounce, at most one rebuild every 2 s), and
   published as a new revision with a delta.

The graph builder is unchanged: it is handed a client whose list calls are
answered from the cache (`internal/engine/cachedclient.go`), so watch and poll
mode produce identical graphs (tested). Before storing, every object has
`managedFields` and `last-applied-configuration` removed, and Secrets have
their `data`/`stringData` removed — Secret values never enter Mantis's memory.

Measured on a synthetic 10,000-resource cluster: poll mode issues 1,259 list
calls every pass; watch mode issues 52 lists and 26 watches once, then none.
On a real cluster a new Pod appears in Mantis about a second after it is
created.

Watch mode needs the read-only `watch` verb (the Helm chart grants it). If the
API server allows list but refuses watch, or the cache cannot sync after three
attempts, the engine falls back to `poll` on its own and says so in its logs
and in `/api/sync/status` (`"mode"`). `MANTIS_SYNC_MODE=poll` selects the old
periodic re-list explicitly (for RBAC that only allows per-namespace reads).

### Revisions and deltas

The engine's background sync publishes a snapshot when a pass completes. The
store (`internal/engine/cache.go`) diffs it against the previous one: identical
content keeps the revision; otherwise the revision increments and a delta (upserted
nodes, removed nodes, added/removed edges) is kept, 64 revisions deep.

```
GET /api/graph               full snapshot (backward compatible), meta.revision set
GET /api/graph?since=<rev>   {"unchanged": true} | {"delta": {...}} | full snapshot if too old
GET /api/graph/search?q=     indexed search: name terms, kind:Pod, ns:shop, label:app=web
GET /api/graph/neighbors?id=&direction=in|out|both&depth=1..4&categories=storage,...
```

Responses are gzip-compressed when the client accepts it. After the first
pass completes, partial (mid-pass) snapshots are no longer published, so a
re-sync never makes the rest of the cluster briefly disappear.

## 2. Projections (UI, `graph-engine.js`)

A projection decides, for the current view, which group each resource is
drawn in, what is folded, and which edges are drawn — without touching the
canonical graph. Every resource is drawn **at most once**, in exactly one
group; a second membership becomes a badge (a Pod shows `on worker-03` in the
namespace view and `ns catalog` in the node view).

| view | groups | notes |
|---|---|---|
| Namespaces | one per namespace + "cluster infrastructure" | cluster-scoped resources never go in a namespace; scheduling is a badge, not edges |
| Nodes | one per worker node (+ unscheduled) | Pods only, namespace as badge; busy nodes fold Pods per namespace |
| Workloads | one per top-level workload | a resource used by exactly one workload joins it; shared ones go to `shared · <ns>`; cluster-scoped to infrastructure |
| Dependencies | the selected resource's neighbourhood | its ownership chain is one unit; outgoing and incoming relationships are walked separately (1–3 hops) |

**Progressive disclosure.**

- Level 1, overview: on a cluster of more than 160 resources, every
  namespace opens as a summary card showing resource count, Pod health,
  broken references and composition. The lines between cards are bundles,
  labelled with how many relationships each one stands for.
- Level 2, a group expanded: inside a group, more than 5 Pods under one
  owner fold into "N Pods", and more than 16 leaves of one kind fold into
  "N ConfigMaps". Click a fold to open it.
- Level 3, one resource: selecting a resource highlights its neighbourhood
  and opens the inspector. The Dependencies view isolates it.

**Hubs.** When a card would have more than 8 lines of one category, those
lines are summarized on the card as `→ 48 Pods` instead of being drawn. A
namespace-wide NetworkPolicy, or a ConfigMap that every Pod mounts, is the
typical case. Ownership is never summarized. Selecting the hub still
highlights every target, and the inspector lists them all.

Filtering by relationship category, namespace or kind changes only the
projection.

## 3. Layout (UI, `graph-engine.js`)

There is **no force simulation** anywhere. Positions are a pure function of
the projection:

1. **Inside a group**, each connected component (typically one application)
   gets a layered layout: edges are oriented by meaning (owner → owned, user →
   used), ranks by longest path with sources pulled next to their targets,
   long edges get dummy waypoints, rank order is improved by barycenter sweeps
   (keeping the ordering with fewest crossings), and vertical positions are
   the least-squares fit to neighbours under order and spacing constraints
   (isotonic regression). Cards with no line inside the group form a grid.
   Components are shelf-packed toward a wide aspect ratio.
2. **Groups** are shelf-packed the same way; in the namespace view cluster
   infrastructure is a column on the right.
3. **Cross-group edges** are routed orthogonally through empty space only:
   card → its column's channel → the gutter between block rows → the group's
   margin → the gutter between group rows → (a margin highway between
   distant rows) → into the target the same way. Parallel edges get lanes;
   edges meeting a card get separate ports.

The same input always produces the same coordinates. A group's interior is
cached by its content, so a change in one namespace re-lays out only that
namespace; elsewhere, cards keep their exact position within their group.
The test suite checks no-overlap, no-line-through-a-card and determinism.

## 4. Rendering (UI, `graph-view.js`)

SVG, with viewport culling (only items within 1.6× the viewport are in the
DOM) and zoom levels of detail (card text drops out when zoomed far out).
The application shell mounts it on the Topology page and supplies where the
graph comes from. The [Playground](https://mantis.continuumx1.com/mantis-playground/playground.html)
is the same app with `/api/*` answered from a recorded sample cluster
(`internal/web/ui/playground/`).

## Testing and measuring

```bash
go test ./...                                    # backend: resolvers, deltas, read-only, Secret safety
node --test 'test/ui/*.test.js'                  # UI engine: projections, layout invariants
go run ./cmd/mantis-dev -resources 5000 -churn 5s -ui-dir internal/web/ui
                                                 # real engine + UI on a synthetic cluster, no Kubernetes needed
go run ./cmd/mantis-dev -resources 10000 -dump /tmp/g10k.json && node test/ui/bench.js /tmp/g10k.json
go test ./internal/engine -run '^$' -bench .     # backend benchmarks
```

`internal/synth` generates the synthetic clusters as real Kubernetes objects,
so they pass through the same resolvers as a live cluster. Deep links such as
`#view=dependency&focus=<id>&select=<id>` or `#expand=ns:catalog` open a
specific picture, which is useful for bug reports and screenshots.

## Known limitations

- Watch mode watches cluster-wide; an identity that may only read some
  namespaces needs `MANTIS_SYNC_MODE=poll`. The graph is still rebuilt as a
  whole from the cache on change (about 0.1 s at 10,000 resources) rather than
  patched in place.
- Not yet resolved: Gateway API routes, EndpointSlices as their own nodes
  (folded into `serves`), VolumeAttachments, NetworkPolicy peer selectors
  (only `podSelector`), and RBAC subjects other than ServiceAccounts (Users
  and Groups are not resources).
- Single cluster per engine. `meta.clusterId` is reported, but IDs are not
  prefixed with it.
- Layout runs on the main thread. It measured under 200 ms even with every
  group of a 10,000-resource cluster expanded, so a Web Worker has not been
  needed.
