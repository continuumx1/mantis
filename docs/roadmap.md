# Roadmap

Priorities evolve from real use — nothing below is a commitment, and
"Planned"/"Future" are explicitly not scheduled.

## Current (1.0)

- Relationship engine with typed, evidence-classified relationships
- Deterministic topology: namespace, node, workload and dependency views
- Application UI: dashboard, resource tables and details, events, capacity
- Watch-based sync with incremental updates (poll fallback)
- Password-protected PDF infrastructure reports (`mantis-report`)
- Configurable administrator sign-in (bcrypt, no default password)
- Any-distribution support via standard kubeconfig / in-cluster auth
- Helm chart (three services, read-only RBAC, network policies)
- Published multi-arch images on Docker Hub (`cx1tech/mantis`)

## Planned

- Multiple users and roles, OIDC single sign-on, audit log
- Per-user, RBAC-aware visibility
- Gateway API resources
- Scheduled reports

## Future

- Change detection and correlation (Git, GitOps, Helm)
- Broader custom resource (CRD) support

This roadmap mirrors the one in the repo's
[README](https://github.com/continuumx1/mantis#roadmap) — if they ever
drift apart, the README is the one that's stale.
