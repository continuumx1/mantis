# FAQ

**What is Mantis?**
: An open-source, read-only Kubernetes tool that discovers the
  relationships between your resources and draws them as an interactive
  graph — Pods, the Deployments that own them, the Services that route to
  them, and more — instead of leaving you to reconstruct that picture by
  hand.

**Is Mantis production-ready?**
: Not yet. Mantis is in Public Preview: functional and safe to point at a
  real cluster, but its login flow, UI, and interfaces are still evolving
  ahead of a stable release. See [Public Preview notes](security.md#public-preview-notes).

**Does Mantis modify my cluster?**
: No. Mantis is strictly read-only — every Kubernetes API call it makes is
  a `get` or `list`. There is no create, update, patch, or delete path
  anywhere in the codebase.

**What permissions does Mantis require?**
: Read-only `get`/`list` access to the resource kinds it maps, granted to
  its own ServiceAccount via the Helm chart's ClusterRole. It never asks
  for write access to anything. See [Architecture](architecture.md#rbac-what-the-engines-serviceaccount-needs).

**Does Mantis ever see my Secret values?**
: No — this is enforced in code, not just hidden in the UI. Secret values
  are never fetched from the Kubernetes API in the first place, so they
  never reach Mantis's backend or browser. See [Security](security.md).

**What Kubernetes distributions are supported?**
: Any of them — minikube, kind, kubeadm, RKE2, EKS, GKE, AKS. Mantis talks
  to the standard Kubernetes API, never to a specific distro.

**Does Mantis require internet access to run?**
: The engine only needs to reach your cluster's API server. It doesn't
  call out anywhere else at runtime. Pulling the published images/chart
  does need registry access once, unless you build and load them locally.

**How current is the data I'm looking at?**
: As current as the last sync — Mantis polls on an interval you control
  (down to every 3 seconds), it doesn't hold a live `watch` subscription.
  See [Core Concepts](core-concepts.md#a-snapshot-not-a-stream).

**Why doesn't every resource have a health indicator?**
: Today the status ring is Pod-only. Other kinds show readiness as text in
  the Summary tab. Broader health indicators are on the roadmap.

**How can I contribute?**
: Join the [Discord](https://discord.gg/ZTB4eGfCxa), open an issue or pull
  request on [GitHub](https://github.com/continuumx1/mantis), or just use
  it and tell us what's confusing — feedback is a contribution too. See
  [Contributing](contributing.md).
