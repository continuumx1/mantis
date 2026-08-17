# Roadmap

Priorities evolve from real use — nothing below is a commitment, and
"Planned"/"Future" are explicitly not scheduled.

## Current

- Relationship engine with a structured `Context`
- Whole-cluster graph as a two-service web application
- Any-distribution support via standard kubeconfig / in-cluster auth
- Verified dangling-reference detection
- Public-preview login gate
- Helm chart (two Deployments, two Services, read-only RBAC)
- Published multi-arch images on Docker Hub (`cx1tech/mantis`)

## Planned

- CI (build/vet/test/gofmt on every PR)
- Real authentication, delegating to Kubernetes RBAC
- Richer per-resource detail in the UI
- Live updates (watch) instead of snapshot-on-poll

## Future

- Change detection and correlation (Git, GitOps, Helm)
- Broader custom resource (CRD) support

This roadmap mirrors the one in the repo's
[README](https://github.com/continuumx1/mantis#roadmap) — if they ever
drift apart, the README is the one that's stale.
