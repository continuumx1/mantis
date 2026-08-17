# Troubleshooting

**Login redirects back to `/login` immediately after signing in.** Confirm
you're using `admin` / `admin` exactly — the demo credential comparison is
case-sensitive. If it still fails, check `mantis-web`'s logs; a
`502`/proxy error there usually means it can't reach `mantis-engine` at the
configured `MANTIS_ENGINE_URL`.

**The graph is empty, or missing resources you know exist.** Check the
`mantis-engine` logs for RBAC "forbidden" messages — a missing `list`
permission on some kind causes Mantis to silently omit that kind rather
than fail the whole graph. Compare against the ServiceAccount's RBAC
grants (see [Architecture](architecture.md#rbac-what-the-engines-serviceaccount-needs)).

**The sync pill shows "Connection lost."** `mantis-web` can't currently
reach `mantis-engine`, or `mantis-engine` can't currently reach the
Kubernetes API. The UI keeps showing the last good graph (with a staleness
timestamp) and keeps retrying — no action needed unless it doesn't
recover.

**A resource's YAML tab shows a 403.** Either it's a Secret (expected — see
[Security](security.md)), or the engine's ServiceAccount lacks `get` RBAC
on that specific kind even though it had `list` (some clusters split
these).

**`ImagePullBackOff` after a Helm install.** Most often means you're
pointed at a locally-built image tag that was never pushed anywhere the
cluster can reach — see [Deployment](deployment.md#container-images) for
loading images into minikube/kind directly, or set `image.pullPolicy` to
match how you're actually distributing the image.

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
  alongside real authentication (see [Roadmap](roadmap.md)).

## Still stuck?

Ask in the [Mantis Discord](https://discord.gg/ZTB4eGfCxa) or open an issue
on [GitHub](https://github.com/continuumx1/mantis/issues) — include what
you ran, what you expected, and the relevant `kubectl logs` output from
whichever service is misbehaving.
