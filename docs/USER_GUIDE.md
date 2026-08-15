# Mantis — User Guide (Public Preview)

This is the end-to-end guide to Mantis: what it is, how it's built, how it
runs inside Kubernetes, and how to use the UI. If you're evaluating Mantis
for the first time or rolling it out for others to try, read this document
top to bottom — it doesn't assume anything beyond basic Kubernetes
familiarity.

> **Status: Public Preview.** Mantis is functional and safe to point at a
> real cluster (it is strictly read-only), but it is early. Interfaces,
> visuals, and the login flow described below will change before a stable
> release. See [Public Preview notes](#public-preview-notes) for exactly
> what that does and doesn't mean.

---

## Table of contents

1. [What is Mantis](#what-is-mantis)
2. [Public Preview notes](#public-preview-notes)
3. [Architecture: how Mantis works end-to-end](#architecture-how-mantis-works-end-to-end)
4. [Security and permissions model](#security-and-permissions-model)
5. [Running Mantis](#running-mantis)
6. [Using the UI](#using-the-ui)
7. [Resource kinds and relationships Mantis understands](#resource-kinds-and-relationships-mantis-understands)
8. [Configuration reference](#configuration-reference)
9. [Known limitations](#known-limitations)
10. [Troubleshooting](#troubleshooting)
11. [Feedback](#feedback)

---

## What is Mantis

Mantis is a **read-only Kubernetes context and investigation tool**. Point
it at a cluster and it draws the relationships between your resources as a
live, interactive graph — Pods, the Deployments that own them, the Services
that route to them, the ConfigMaps and Secrets they mount, the Nodes they
run on, and more — instead of leaving you to reconstruct that picture
yourself from `kubectl get` and `describe` output.

It answers the questions Kubernetes' raw metadata makes you assemble by
hand:

- Why does this Pod exist, and what created it?
- Which Pods does this Service actually route to?
- This Pod is stuck `Pending` — what is it waiting on?
- This Ingress returns a 5xx — does the Service it points at even exist?

Every resource is a node, every relationship is a typed, directional edge,
and namespaces are drawn as visual regions on the canvas. A reference that
points at something Mantis checked and could not find (a Service selecting
Pods that don't exist, a PVC bound to a missing PV) is drawn as a **dashed,
"not found"** edge — Mantis distinguishes what it verified from what it
merely read off a spec field, and never guesses.

Mantis is not a replacement for `kubectl`, k9s, a metrics stack, or a GitOps
tool — it does one job (explain how what's running relates to what else is
running) and stays out of the way of the tools that already do the others.

---

## Public Preview notes

Mantis is being released as a **Public Preview** so people can try it
against real clusters and give feedback before the interfaces settle. A few
things are true about this stage specifically, and worth knowing before you
hand the URL to anyone else:

- **The login screen is a temporary demo gate, not real authentication.**
  There is a single hardcoded credential (`admin` / `admin`) protecting the
  UI — it exists only so the graph isn't sitting wide open on a URL, not as
  an access-control system. It will be replaced by real authentication
  (almost certainly delegating to Kubernetes RBAC via your own identity)
  before Mantis leaves preview. Do **not** expose a Public Preview
  deployment on the open internet, and don't treat `admin`/`admin` as a
  credential you're expected to change per-deployment — it's a fixed demo
  password, not meant to become a default that someone accidentally ships.
- **What visibility you get is still your Kubernetes permissions, not
  Mantis's** — see [Security and permissions model](#security-and-permissions-model).
  Mantis does not add an authorization layer on top of Kubernetes; the
  Mantis backend reads with a single ServiceAccount's permissions, and
  everyone using that Mantis deployment currently sees the same graph that
  ServiceAccount can see.
- **The UI and layout will keep changing.** Screens described in this guide
  are accurate as of this preview build but are not a stable contract.
- Mantis is **strictly read-only against the cluster** — this part is not
  going to change. It never creates, updates, patches, or deletes anything.
  It only calls `get`/`list`/`watch`-shaped read APIs.

---

## Architecture: how Mantis works end-to-end

Mantis runs as **two independent services** in your cluster, plus your
browser as the client. There is no database, no separate storage, and no
third component — every graph you see is built live from a fresh read of
the Kubernetes API.

```
 Your browser
      │  HTTPS/HTTP, same-origin (no CORS)
      ▼
 ┌───────────────┐   reverse proxy    ┌───────────────┐   client-go /    ┌──────────────────┐
 │  mantis-web   │  ── /api/* ──────► │ mantis-engine │ ── dynamic ────► │ Kubernetes API   │
 │  (frontend)   │                    │  (backend)    │    client        │  server          │
 │               │ ◄── graph JSON ─── │               │ ◄── objects ──── │                  │
 └───────────────┘                    └───────────────┘                  └──────────────────┘
   Deployment,                          Deployment,
   usually exposed via                  ClusterIP only —
   Ingress/LoadBalancer                 never reachable
                                        from outside the
                                        cluster
```

### The two services

- **`mantis-engine` (backend)** — the only service that talks to
  Kubernetes. On startup it builds a client using its Pod's ServiceAccount
  token (the standard in-cluster config); outside a cluster it falls back
  to your local kubeconfig, so the exact same binary works as a deployed
  service or as a local dev tool. It exposes `GET /api/graph` (the whole
  cluster, as JSON) and `GET /api/resource` (one resource's YAML manifest,
  fetched on demand when you open it in the UI), plus `/healthz` and
  `/readyz` probes. It renders no UI.
- **`mantis-web` (frontend)** — serves the embedded single-page UI and
  reverse-proxies every `/api/*` request straight through to
  `mantis-engine`. Your browser only ever talks to `mantis-web`; it never
  makes a direct request to the engine or to the Kubernetes API. This is
  also what lets `mantis-engine` stay a private `ClusterIP` service with no
  public exposure at all — only `mantis-web` needs an Ingress or
  LoadBalancer.

### What happens on a page load

1. You open the Mantis URL. `mantis-web` checks for a valid session cookie;
   if you don't have one, you're redirected to `/login`.
2. After signing in, the browser loads the single-page app (one HTML
   document with its JS/CSS inlined — no separate asset requests, no CDN).
3. The page calls `GET /api/graph`. `mantis-web` proxies this straight to
   `mantis-engine`.
4. `mantis-engine` walks every namespace (plus cluster-scoped kinds) using
   its Kubernetes client, issuing `list` calls for each resource kind it
   understands (see [Resource kinds](#resource-kinds-and-relationships-mantis-understands)),
   resolves the relationships between what it found (owner references,
   selectors, endpoint slices, volume/env references, …), and assembles
   an in-memory graph.
5. That graph is serialized to JSON and returned. The browser never sees
   raw Kubernetes objects for the graph view — only the derived nodes,
   edges, and the compact display attributes the engine computed
   (readiness, image, probes, resource requests, …).
6. The UI renders the graph as SVG and starts a force-directed layout
   simulation client-side (no server involvement) so nodes settle into
   place and you can drag them around.
7. On an interval you control (see [the sync pill](#staying-in-sync-the-sync-pill)),
   the browser repeats step 3 and re-renders — this is a poll, not a
   push/watch subscription, so what you see is a snapshot as of the last
   sync, not a live stream.
8. If you click a resource and open its **YAML** tab, *that* triggers a
   separate, single, on-demand `GET /api/resource?kind=...&name=...&ns=...`
   call — the engine fetches that one object fresh from the Kubernetes API
   (via the dynamic client, so this also covers CRDs), strips
   server-managed noise (`managedFields`), and returns it as YAML. Nothing
   about YAML fetching happens unless you explicitly open that tab.

### RBAC: what the engine's ServiceAccount needs

`mantis-engine`'s ServiceAccount needs **read-only** (`get`/`list`) access
to the resource kinds it maps — core workloads, networking, storage,
autoscaling objects, Nodes, and (best-effort, only if installed)
VerticalPodAutoscaler and Karpenter NodePool custom resources. It needs no
write verbs on anything, ever — there is no code path in Mantis that issues
a `create`, `update`, `patch`, or `delete` call. If a `list` for some kind
fails with a Forbidden error (RBAC denies it), Mantis doesn't error out the
whole graph: it silently omits that kind and the graph reflects only what
the ServiceAccount could actually read. Mantis's own visibility is a
direct, honest reflection of the permissions it was granted — nothing more.

There is no Helm chart shipping yet (it's on the near-term roadmap); until
then, deploy the two images with your own Deployment/Service/RBAC
manifests, sized to your cluster's namespace count. See
[Running Mantis](#running-mantis) for the container images and the exact
environment variables each service reads.

---

## Security and permissions model

This section exists because "read-only" and "safe" get used loosely — here
is specifically what Mantis does and does not do.

- **Mantis never mutates the cluster.** Every Kubernetes API call it makes
  is a read (`get`/`list`; the dynamic client for CRDs and single-resource
  YAML fetches). There is no create/update/patch/delete path anywhere in
  the codebase.
- **Access follows your Kubernetes permissions, not the other way
  around.** Mantis doesn't introduce its own authorization model on top of
  Kubernetes. What the graph shows is bounded by what the `mantis-engine`
  ServiceAccount's RBAC grants allow it to read — scope that ServiceAccount
  the way you'd scope any read-only tooling account.
- **Secret values are never fetched, not just never displayed.** This is a
  structural guarantee, not a UI-layer filter:
  - The `GET /api/resource` YAML endpoint refuses `kind=Secret` outright,
    *before* issuing any API call — a Secret's contents never leave the
    Kubernetes API server, let alone reach the Mantis backend or your
    browser.
  - When the graph builder lists Secrets (to draw them as nodes and derive
    relationships like "this Pod mounts this Secret"), it reads only
    `Name` and `Type`, and explicitly zeroes out the `Data`/`StringData`
    fields on the in-memory object immediately after listing — before
    anything downstream (an attribute builder, an error message, a log
    line) has a chance to touch them.
  - The UI shows Secret nodes (so you can see *that* something depends on
    a Secret and *which* one) but its YAML/detail view always reads:
    *"Secret contents are hidden for security. Mantis never displays Secret
    data through the UI."*
- **System-managed noise can be hidden.** By default, Mantis hides
  Kubernetes- and Helm-managed clutter that isn't useful for understanding
  your workloads — e.g. the `kube-root-ca.crt` ConfigMap every namespace
  gets automatically, and system-generated Secrets. Set
  `MANTIS_SHOW_ALL=true` on the engine if you want to see everything
  instead.
- **The login gate is a preview-only convenience, not a security
  boundary** — see [Public Preview notes](#public-preview-notes). Don't
  rely on it to keep out anyone you don't already trust with
  `mantis-engine`'s RBAC-granted read access.

---

## Running Mantis

### Container images

Each service builds from its own multi-stage, distroless, nonroot
Dockerfile:

```bash
docker build -f build/Dockerfile.engine -t mantis-engine:dev .
docker build -f build/Dockerfile.web    -t mantis-web:dev .
```

### Deploying

Run `mantis-engine` as a Deployment with a ServiceAccount that has
read-only RBAC for the kinds it maps (see [RBAC](#rbac-what-the-engines-serviceaccount-needs)),
exposed only as a `ClusterIP` Service — it should never be reachable from
outside the cluster. Run `mantis-web` as a Deployment pointed at that
Service via `MANTIS_ENGINE_URL`, exposed through whatever Ingress or
LoadBalancer fits your cluster.

There's no Helm chart yet, so today that means hand-writing (or generating)
the two Deployments, two Services, and the RBAC objects. A chart is the
next packaging step on the roadmap.

### Running locally (for evaluation)

Requires Go 1.26+ and a reachable cluster (any distribution — the client
just needs a working kubeconfig):

```bash
git clone https://github.com/continuumx1/mantis.git
cd mantis

# Terminal 1 — backend, reads the cluster via your kubeconfig
MANTIS_ENGINE_ADDR=":8080" go run ./cmd/mantis-engine

# Terminal 2 — frontend, serves the UI and proxies /api to the engine
MANTIS_WEB_ADDR=":8081" MANTIS_ENGINE_URL="http://127.0.0.1:8080" go run ./cmd/mantis-web
```

Open `http://127.0.0.1:8081`, sign in with `admin` / `admin`, and you get
the live graph of whatever cluster your kubeconfig points at.

---

## Using the UI

### Signing in

The Public Preview build sits behind a login screen. Sign in with
**`admin` / `admin`** (the credential is printed on the login screen itself
— see [Public Preview notes](#public-preview-notes) for what this
credential is and isn't). The session lasts 12 hours or until you sign out.

### The graph canvas

Once signed in, you land on the main graph:

- **Namespace regions.** Each namespace is drawn as a soft, labeled region
  on the canvas; cluster-scoped resources (Nodes, NodePools, …) get their
  own region rather than being forced into a namespace they don't belong
  to. Regions are boundaries for orientation, not hard walls — you can drag
  a node anywhere.
- **Resource cards (nodes).** Each card shows the resource's kind, name,
  and — for Pods — a small status ring: **green** means running/succeeded
  with every container ready; **red** means anything else (crash-looping,
  pending, not-ready containers, …). Other kinds don't carry a health ring
  today (see [Known limitations](#known-limitations)).
- **Edges.** A line between two cards is a relationship, drawn as a solid
  line when it points at something that exists, or a **dashed red** line
  when Mantis checked and the target isn't actually there (a dangling
  reference). Hover the legend (bottom-left "i" icon) for the full color
  key — categories (Workload, Pod, Service/Ingress, Config/Secret, Storage,
  Node, Autoscaler) and edge meanings.
- **Pan, zoom, drag.** Scroll/pinch to zoom, drag empty canvas to pan, drag
  a card to reposition it (it stays pinned where you drop it until you
  reload).

### Selecting a resource: the details drawer

Click any card to open the **details drawer** on the right. It has three
tabs:

- **Summary** — the compact, human-readable facts the engine computed for
  that resource: readiness, image, resource requests/limits, probes,
  storage class, endpoints, and similar, kind-specific detail. Anything
  meaningfully absent (no probes configured, no resource requests set) is
  shown as `none` rather than omitted — the gap is often the thing you're
  looking for.
- **Relationships** — every edge touching this resource, in both
  directions, each one clickable to jump straight to the other end.
- **YAML** — the resource's live manifest, fetched fresh from the cluster
  the moment you open this tab (server-managed `managedFields` noise is
  stripped). Secrets show the "hidden for security" message instead of a
  manifest — see [Security and permissions model](#security-and-permissions-model).
  The YAML view has its own **find-in-page search** (the same `Ctrl F`
  shortcut, scoped to the open manifest) with match highlighting and
  next/previous navigation.

Close the drawer with the ✕ in its header, or press `Escape`.

### Relationship focus and tracing

Selecting a resource doesn't just open its drawer — it also **traces**
outward through the graph a couple of hops and dims everything else: direct
neighbors of the selected resource stay fully visible, resources a couple
of hops further out are dimmed but still legible, and everything unrelated
fades into the background. This is the fastest way to answer "what does
this actually connect to" without hunting across a busy canvas. Selecting
nothing (click empty canvas, or `Escape`) clears the trace and restores
everything.

### Search

Click the search box (or press `Ctrl F`, from anywhere) to search by
resource name or kind. Selecting a result both selects that resource
(opening its drawer and trace) and smoothly flies the camera to center it
on screen — you don't have to manually hunt for it on a large graph.
`Ctrl F` works the same on Windows, Linux, and macOS — it's not
Cmd-only or platform-specific.

### Staying in sync: the sync pill

The pill near the top of the screen shows both *that* Mantis is syncing
with the cluster and *how often*: it always displays the current interval
next to its status dot (`3s`, `10s`, `30s`, `1m`, `5m`), never a vague
"Live" label with no number attached. Click it to change the interval or
switch to **Manual** (no automatic refresh; use the Refresh control
instead). The fastest automatic interval is 3 seconds — anything faster
isn't offered, since polling a whole-cluster read on a very tight loop
isn't a good default for API server load. If the engine can't be reached,
the pill switches to a "Connection lost" state and shows how stale the
displayed data is while it keeps retrying.

Reopening a resource's drawer or YAML tab is preserved across a background
sync — an automatic refresh updates the graph underneath you without
closing whatever you had open.

### Theme

Mantis opens in **dark theme by default**. Toggle to light with the
sun/moon control in the header if you prefer it; the choice is remembered
for the rest of that browser tab (so a reload doesn't flip back to dark
under you) but is not carried over to a new tab or the next time you open
Mantis — it always starts dark.

### Signing out

The **Sign out** control in the header ends your session immediately and
returns you to the login screen.

### Keyboard shortcuts

| Shortcut | Action |
|---|---|
| `Ctrl F` | Focus search (or the YAML find-in-page box, if a YAML tab is open) |
| `Escape` | Close YAML search → close details drawer → clear selection (in that order, one press at a time) |
| `Enter` / `Shift Enter` | Next / previous match, inside YAML find-in-page |
| Scroll / pinch | Zoom the canvas |
| Drag (empty canvas) | Pan |
| Drag (a card) | Reposition that resource |

---

## Resource kinds and relationships Mantis understands

### Resource kinds

Core, built-in kinds: `Pod`, `Node`, `Service`, `Ingress`, `ConfigMap`,
`Secret` (metadata only — see [security](#security-and-permissions-model)),
`PersistentVolumeClaim`, `PersistentVolume`, `ResourceQuota`,
`LimitRange`, `Deployment`, `ReplicaSet`, `StatefulSet`, `DaemonSet`,
`Job`, `CronJob`, `HorizontalPodAutoscaler`.

Optional, best-effort custom resources (included automatically if the CRD
is installed and RBAC allows reading it; silently omitted otherwise, never
an error): `VerticalPodAutoscaler`, Karpenter `NodePool`. Mantis also
detects — and notes in the header — whether a cluster-level node
autoscaler (Karpenter or the classic `cluster-autoscaler`) appears to be
running, without requiring either to be present.

### Relationship types

| Relationship | Meaning | Derived from |
|---|---|---|
| `controlled-by` | A resource is controlled by its owner | `ownerReferences` |
| `runs-on` | A Pod is scheduled onto a Node | `pod.spec.nodeName` |
| `selects` | A Service selects Pods | `service.spec.selector` |
| `serves` | A Service actually backs a Pod | EndpointSlices |
| `routes-to` | An Ingress routes to a Service | ingress backends |
| `references` | A Pod consumes config via the environment | `env` / `envFrom` |
| `mounts` | A Pod mounts config as a volume | `volumes` |
| `claims` | A Pod claims a PersistentVolumeClaim | `volumes[].persistentVolumeClaim` |
| `bound-to` | A PVC is bound to a PersistentVolume | `pvc.spec.volumeName` |
| `scales` | An autoscaler (HPA/VPA) drives a workload's replica count or resources | HPA/VPA `scaleTargetRef` / `targetRef` |

Every one of these is verified where possible: if what a relationship
points at was checked and doesn't exist, Mantis draws that edge as
**not found** rather than pretending the reference resolved.

---

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

---

## Known limitations

- **Snapshot-on-poll, not push.** The graph reflects the cluster as of the
  last sync (as fast as every 3 seconds, or manual); there's no live
  watch/stream yet, so a change can be up to one sync interval old.
- **Pod-only health today.** The status ring only reflects Pod health;
  other kinds (Deployments, StatefulSets, …) show their readiness as text
  in the Summary tab but don't yet carry a ring of their own.
- **Built-in kinds plus two specific CRDs, not arbitrary CRDs.** VPA and
  Karpenter NodePool are understood specifically; other custom resources
  aren't mapped into the graph yet.
- **No change history or GitOps awareness.** Mantis explains current
  state, not what changed, when, or why.
- **Single shared visibility per deployment.** Everyone using one Mantis
  deployment currently sees the same graph — the one `mantis-engine`'s
  ServiceAccount can read. Per-user, RBAC-aware visibility is planned
  alongside real authentication.
- **No Helm chart yet** — deployment today means authoring your own
  manifests (see [Running Mantis](#running-mantis)).

## Troubleshooting

**Login redirects back to `/login` immediately after signing in.**
Confirm you're using `admin` / `admin` exactly — the demo credential
comparison is case-sensitive. If it still fails, check `mantis-web`'s logs;
a `502`/proxy error there usually means it can't reach `mantis-engine` at
the configured `MANTIS_ENGINE_URL`.

**The graph is empty, or missing resources you know exist.** Check the
`mantis-engine` logs for RBAC "forbidden" messages — a missing `list`
permission on some kind causes Mantis to silently omit that kind rather
than fail the whole graph. Compare against the ServiceAccount's RBAC
grants.

**The sync pill shows "Connection lost."** `mantis-web` can't currently
reach `mantis-engine`, or `mantis-engine` can't currently reach the
Kubernetes API. The UI keeps showing the last good graph (with a
staleness timestamp) and keeps retrying — no action needed unless it
doesn't recover.

**A resource's YAML tab shows a 403.** Either it's a Secret (expected —
see [security](#security-and-permissions-model)), or the engine's
ServiceAccount lacks `get` RBAC on that specific kind even though it had
`list` (some clusters split these).

## Feedback

Mantis is in Public Preview specifically to collect feedback before its
interfaces lock in. If something is confusing, missing, or broken, that's
useful signal — please report it rather than working around it silently.
