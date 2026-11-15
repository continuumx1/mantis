# Troubleshooting

**"Invalid username or password".** The account is the one configured at
install time; both fields are case-sensitive. With Helm, read the password
from the admin Secret, `mantis-admin` for a release named `mantis` (see
[Getting Started](getting-started.md#first-login)). After 10 failures from
one address, sign-in is refused for 10 minutes ("too many failed
sign-ins").

**`mantis-web` exits at startup with "no admin password configured".** Set
`MANTIS_ADMIN_PASSWORD_HASH` or `MANTIS_ADMIN_PASSWORD` (the Helm chart does
this for you).

**Signed out unexpectedly.** Sessions live in `mantis-web`'s memory: a
restart or upgrade signs everyone out, and with more than one `mantis-web`
replica you need sticky sessions on your Ingress.

**Login redirects back to `/login` immediately after signing in.** Check
`mantis-web`'s logs; a `502`/proxy error there usually means it can't reach
`mantis-engine` at the configured `MANTIS_ENGINE_URL`.

**"Download report" says reports are not available.** `mantis-report` is
not deployed (`report.enabled=false`) or `MANTIS_REPORT_URL` is unset. A
"try again in a moment" message means that replica is already generating
`report.maxConcurrent` reports.

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

- **Refresh interval.** The engine sees changes within about a second,
  but the browser asks for them every 10 seconds by default (configurable),
  so the screen can be up to one interval behind.
- **No Gateway API yet.** Gateways and HTTPRoutes are not mapped.
- **Built-in kinds plus two specific CRDs, not arbitrary CRDs.** VPA and
  Karpenter NodePool are understood specifically; other custom resources
  aren't mapped into the graph yet.
- **No change history or GitOps awareness.** Mantis explains current
  state, not what changed, when, or why.
- **One administrator account, shared visibility.** Everyone signed in
  sees the same graph — the one `mantis-engine`'s ServiceAccount can read.
  Multiple users, roles, SSO and per-user, RBAC-aware visibility are
  planned (see [Roadmap](roadmap.md)).

## Still stuck?

Ask in the [Mantis Discord](https://discord.gg/ZTB4eGfCxa) or open an issue
on [GitHub](https://github.com/continuumx1/mantis/issues) — include what
you ran, what you expected, and the relevant `kubectl logs` output from
whichever service is misbehaving.
