# Application UI

Mantis has two kinds of pages. The **Topology** page is the graph: its own
renderer, its own interaction model (see [the graph architecture](graph-architecture.md)).
**Everything else** — dashboard, resource tables, resource details, events,
administration — is the application shell in `internal/web/ui/app-shell.js` and
`app-shell.css`. No framework and no build step; the files are embedded in
`mantis-web`.

## Pages

| Route | Page | Data |
|---|---|---|
| `#/dashboard` | Cluster overview, health, pod status, workloads by namespace, pods per node, capacity, recent warnings, needs-attention list | `/api/graph`, `/api/events`, `/api/cluster/capacity` |
| `#/topology` | The graph (graph-view.js, mounted unchanged and kept alive across pages) | `/api/graph` |
| `#/r/<kind>` | Resource table: filter, status filter, sort, pagination | `/api/graph` |
| `#/o/<id>` | Resource detail: Overview, YAML, Events, Conditions, Related resources | `/api/graph`, `/api/resource`, `/api/events` |
| `#/namespaces`, `#/events` | Namespace health; cluster events | `/api/graph`, `/api/events` |
| `#/access`, `#/settings` | What sign-in and cluster access actually are; theme and refresh preferences | `/api/graph` meta |

Every number comes from the engine. Capacity *usage* is shown only when
metrics-server is installed; otherwise the dashboard says so and shows
allocatable capacity only. The Access page describes the Public Preview sign-in
as it is — there are no user, role or OIDC controls until those exist in the
backend. Secrets never show values, YAML or annotations.

The engine endpoints added for these pages are read-only and cached briefly
(`/api/events` 10 s, `/api/cluster/capacity` 15 s); `/api/resource?format=json`
returns a manifest as JSON for the Conditions and Annotations views. The Helm
chart grants `list` on `events` and on `metrics.k8s.io` `nodes`.

## Design tokens

All colors, sizes and radii are CSS custom properties on `:root[data-theme]`
in `app-shell.css`, defined separately for light and dark. Pages never
hardcode a color. The shell also maps the topology's own tokens (`--ground`,
`--panel`, `--brand`, …) onto the same palette, so the graph matches the shell
without any change to its renderer.

| Role | Light | Dark |
|---|---|---|
| Sidebar | `#111827` / `#1F2937` | `#0A0F1A` / `#151D2B` |
| Background / surface | `#F5F7FA` / `#FFFFFF` | `#0D1421` / `#141C2B` |
| Text / secondary | `#172033` / `#64748B` | `#E6EAF0` / `#98A4B6` |
| Border | `#E2E8F0` | `#243045` |
| Accent (Mantis teal) | `#0D9488`, hover `#0F766E`, soft `#CCFBF1` | `#14B8A6` |
| Status | ok `#16A34A`, warn `#D97706`, critical `#DC2626`, info `#2563EB`, unknown `#94A3B8` | lighter tints of the same |

Shell classes are prefixed `m-` so they never collide with the topology's
stylesheet. Status always uses the same vocabulary through one function
(`statusOf`), so a Pod or Deployment reads the same in a table, a detail
page and the dashboard.

## Typography

Inter for the interface, JetBrains Mono only for technical values (resource
names, YAML, IPs, versions, CLI text). Both are bundled in
`internal/web/ui/fonts/` (SIL Open Font License, licence files alongside), so
nothing is fetched from a CDN — Mantis often runs on clusters without internet.

| Use | Size / weight |
|---|---|
| Page title | 20px / 600 |
| Section (card) heading | 15px / 600 |
| Body, tables, sidebar | 13px / 400–500 |
| Technical values (mono) | 12.5px |
| Secondary metadata | 11.5–12px |
| Graph card: kind / name / badge | 10px Inter 600 / 12px Mono 500 / 10.5px Inter |
| Graph card zoomed out | one 15px Inter name replaces the three lines |

## Branding

`internal/web/ui/brand/`: `mantis-mark.svg` (the compact symbol, used in the
sidebar), `mantis-logo-light.svg` / `mantis-logo-dark.svg` (symbol + wordmark
for light and dark backgrounds) and `favicon.svg`. The mascot illustration
(`mantis-mascot.webp`) stays for the login screen, docs and community material
rather than every UI surface.

## Topology presentation

- Legend: a compact strip, bottom-left, open by default; it steps aside while
  the inspector is open.
- Status bar: what is drawn, what is in collapsed groups, folded into stacks,
  or filtered out — each count is a button that undoes that hiding.
- Hovering a line names the relationship, its evidence class and the API field
  it was read from. Counts ("3 links") appear only on bundles between groups.

## Layout

Collapsible dark sidebar (icon rail below 1100 px, a drawer below 760 px), a
top bar with cluster, namespace selector (applies to every page, including
Topology), global search (`Ctrl K` or `/`; `kind:`, `ns:`, `label:` filters),
sync status and theme. Keyboard: tables rows are focusable and open with
Enter; every control has a visible focus ring.
