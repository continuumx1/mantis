# Changelog

All notable changes to Mantis. Versions follow [Semantic Versioning](https://semver.org/).

## 1.0.0 – Vishwakarma · The Divine Architect

The first stable release. Mantis is now three services — `mantis-web`,
`mantis-engine` and `mantis-report` — and leaves Public Preview.

The release is named after Vishwakarma, the divine architect and master
craftsman in Indian tradition: a dedication to the engineers, architects and
open-source builders who create the invisible foundations of our digital
world. See the [release notes](docs/releases/v1.0.0.md).

### Upgrading from 0.3.0

- **Upgrade with new defaults.** Use `helm upgrade --reset-then-reuse-values`
  (Helm 3.14+) or pass your values file. Plain `--reuse-values` reuses the
  0.3.0 values, which lack the new `auth` and `report` settings, and the
  upgrade fails.
- **Sign-in changed.** The hardcoded `admin`/`admin` login is gone. The
  chart now generates a random administrator password into a Secret (or
  uses `auth.adminPassword` / `auth.existingSecret`); the install notes
  print the command to read it. For a release named `mantis`:
  `kubectl get secret mantis-admin -n <namespace> -o jsonpath='{.data.password}' | base64 -d`.
  Without the chart, set `MANTIS_ADMIN_PASSWORD_HASH` (bcrypt) or
  `MANTIS_ADMIN_PASSWORD`; `mantis-web` will not start without one.
- **A third image**, `cx1tech/mantis:1.0.0-report`, is deployed by default
  (`report.enabled`).
- **RBAC** adds the read-only `watch` verb (for watch-based sync), `list`
  on `events`, and `metrics.k8s.io` `nodes`. Still no write verbs.

### Topology

- A deterministic, projection-based graph replaces the force-directed
  layout: the same cluster always produces the same picture, with no
  overlapping cards and no lines through cards.
- Four views: Namespaces, Nodes, Workloads and Dependencies (1–3 hops).
- Typed relationships (ownership, networking, configuration, storage,
  security, autoscaling, scheduling), each with its evidence and source
  field; dangling references drawn as dashed red lines.
- Progressive disclosure on large clusters: namespace summary cards,
  folded stacks ("48 Pods"), hub summaries; a status bar that undoes each
  kind of hiding.
- Movable zoom bar (`+`, `−`, slider, Fit), legend, find-in-graph
  (`Ctrl F`).

### Application UI

- New application shell: sidebar, namespace selector applied to every page,
  global search (`Ctrl K` or `/`, with `kind:`, `ns:`, `label:` filters).
- Dashboard: health, Pod status, workloads by namespace, Pods per node,
  capacity (usage with metrics-server), recent warnings, needs-attention.
- Resource tables with filters, sorting and paging; resource details with
  Overview, YAML, Events, Conditions and Related resources.
- Namespaces, Events, Access and Settings pages; light and dark themes.
- Bundled Inter and JetBrains Mono fonts (no CDN); keyboard shortcuts shown
  for the visitor's platform.
- The Mantis mascot is now the logo everywhere: vector mark, favicon and
  wordmark logos.

### Engine

- Watch-based sync: one list per kind, then Kubernetes watches feed an
  in-memory cache; the graph is rebuilt from the cache when changes arrive.
  Falls back to periodic re-listing if watches are refused
  (`MANTIS_SYNC_MODE`).
- Revisioned graph with deltas: `GET /api/graph?since=<revision>` returns
  only what changed.
- New read-only endpoints: `/api/events`, `/api/cluster/capacity`,
  `/api/sync/status`, `/api/graph/search`, `/api/graph/neighbors`.
- Internal `GET /internal/v1/snapshot` for `mantis-report` (not proxied to
  browsers); Secret data, ConfigMap values and managed fields are stripped.

### Infrastructure reports (new service: `mantis-report`)

- **Download report** produces a PDF with cover, clickable contents and
  bookmarks, overview, health, vector topology diagrams sized to their
  content (overview plus per-namespace pages for large clusters),
  inventory, findings and metadata.
- AES-256 encryption (PDF 2.0, revision 6) with a password the user
  chooses; the password is never stored or logged.
- Every resource and relationship on a diagram is checked against the
  engine's snapshot. Stateless and horizontally scalable; streamed
  progress; cancellable.

### Security

- Configurable administrator account: bcrypt-hashed password, no default.
- Failed sign-ins limited per client address (10 per 10 minutes).
- Session cookies marked `Secure` over HTTPS (including behind a
  TLS-terminating proxy).
- `mantis-report` runs without a ServiceAccount token; network policies
  cover web → engine, web → report and report → engine.

## 0.3.0

Static, no-backend Playground demo and the public docs site; neutral
charcoal-grey UI theme. Image `0.1.0-preview.3`.

## 0.2.0

Dependency updates; 0 known vulnerabilities (Docker Scout). Image
`0.1.0-preview.2`.

## 0.1.0

First Public Preview. Image `0.1.0-preview.1`.
