# FAQ

**What is Mantis?**
: An open-source, read-only Kubernetes tool that discovers the
  relationships between your resources and draws them as an interactive
  graph — Pods, the Deployments that own them, the Services that route to
  them, and more — instead of leaving you to reconstruct that picture by
  hand.

**Is Mantis production-ready?**
: Yes, as of 1.0. It is strictly read-only, signs in with an administrator
  password you configure (there is no default), and never handles Secret
  values. It has one administrator account today; multiple users, roles
  and single sign-on are on the [Roadmap](roadmap.md). See
  [Security](security.md#signing-in) for the recommended setup.

**How do I get the admin password?**
: The Helm chart generates one unless you set `auth.adminPassword`. Read
  it with `kubectl get secret mantis-admin -n <namespace> -o jsonpath='{.data.password}' | base64 -d`.

**Can I export what Mantis shows?**
: Yes: **Download report** produces a password-protected (AES-256) PDF
  with the topology, inventory, health and findings. See
  [Infrastructure Reports](reports.md).

**Does Mantis modify my cluster?**
: No. Mantis is strictly read-only — every Kubernetes API call it makes is
  a `get` or `list`. There is no create, update, patch, or delete path
  anywhere in the codebase.

**What permissions does Mantis require?**
: Read-only `get`/`list`/`watch` access to the resource kinds it maps, granted to
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
: Within seconds. The engine follows the cluster with Kubernetes watches
  and picks up a change in about a second; the browser fetches what changed
  every 10 seconds by default (5 seconds to manual, in Settings). See
  [Core Concepts](core-concepts.md#how-mantis-stays-current).

**How does Mantis decide what is healthy?**
: From observed state only: Pod phase and container readiness, workload
  replica availability, node conditions, claim binding, and references that
  point at nothing. The same rules drive the dashboard, resource tables and
  reports.

**How can I contribute?**
: Join the [Discord](https://discord.gg/ZTB4eGfCxa), open an issue or pull
  request on [GitHub](https://github.com/continuumx1/mantis), or just use
  it and tell us what's confusing — feedback is a contribution too. See
  [Contributing](contributing.md).
