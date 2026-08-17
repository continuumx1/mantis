# Security

This page exists because "read-only" and "safe" get used loosely — here is
specifically what Mantis does and does not do.

## The read-only guarantee

Mantis never mutates the cluster. Every Kubernetes API call it makes is a
read (`get`/`list`; the dynamic client for CRDs and single-resource YAML
fetches). There is no create/update/patch/delete path anywhere in the
codebase.

## Access follows your Kubernetes permissions

Mantis doesn't introduce its own authorization model on top of Kubernetes.
What the graph shows is bounded by what the `mantis-engine` ServiceAccount's
RBAC grants allow it to read — scope that ServiceAccount the way you'd
scope any read-only tooling account. See [Architecture](architecture.md#rbac-what-the-engines-serviceaccount-needs)
for exactly what it asks for and why.

Everyone using one Mantis deployment currently sees the same graph — the
one that ServiceAccount can read. There's no per-user visibility yet (see
[Roadmap](roadmap.md)).

## Secret values are never fetched, not just never displayed

This is a structural guarantee, not a UI-layer filter:

- The `GET /api/resource` YAML endpoint refuses `kind=Secret` outright,
  *before* issuing any API call — a Secret's contents never leave the
  Kubernetes API server, let alone reach the Mantis backend or your
  browser.
- When the graph builder lists Secrets (to draw them as nodes and derive
  relationships like "this Pod mounts this Secret"), it reads only `Name`
  and `Type`, and explicitly zeroes out the `Data`/`StringData` fields on
  the in-memory object immediately after listing — before anything
  downstream (an attribute builder, an error message, a log line) has a
  chance to touch them.
- The UI shows Secret nodes (so you can see *that* something depends on a
  Secret and *which* one) but its YAML/detail view always reads: *"Secret
  contents are hidden for security. Mantis never displays Secret data
  through the UI."*

## System-managed noise can be hidden

By default, Mantis hides Kubernetes- and Helm-managed clutter that isn't
useful for understanding your workloads — e.g. the `kube-root-ca.crt`
ConfigMap every namespace gets automatically, and system-generated
Secrets. Set `MANTIS_SHOW_ALL=true` on the engine if you want to see
everything instead.

## Public Preview notes

Mantis is being released as a **Public Preview** so people can try it
against real clusters and give feedback before the interfaces settle. A few
things are true about this stage specifically, and worth knowing before you
hand the URL to anyone else:

!!! danger "The login screen is a temporary demo gate, not real authentication"
    There is a single hardcoded credential (`admin` / `admin`) protecting
    the UI — it exists only so the graph isn't sitting wide open on a URL,
    not as an access-control system. It will be replaced by real
    authentication (almost certainly delegating to Kubernetes RBAC via
    your own identity) before Mantis leaves preview. **Do not expose a
    Public Preview deployment on the open internet**, and don't treat
    `admin`/`admin` as a credential you're expected to change
    per-deployment — it's a fixed demo password, not meant to become a
    default that someone accidentally ships.

- **The login gate is a preview-only convenience, not a security
  boundary.** Don't rely on it to keep out anyone you don't already trust
  with `mantis-engine`'s RBAC-granted read access.
- **The UI and layout will keep changing.** Screens described in this
  guide are accurate as of this preview build but are not a stable
  contract.
- Mantis is **strictly read-only against the cluster** — this part is not
  going to change. It never creates, updates, patches, or deletes
  anything. It only calls `get`/`list`/`watch`-shaped read APIs.

## Reporting a vulnerability

If you find a genuine security issue, please don't open a public GitHub
issue for it — reach out via the [Mantis Discord](https://discord.gg/ZTB4eGfCxa)
instead, so it can be addressed before details are public. A formal
`SECURITY.md` reporting process is on the way; until it lands, Discord is
the reliable path to the maintainers.
