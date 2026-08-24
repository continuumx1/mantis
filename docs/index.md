# Mantis

**Visual Kubernetes topology and relationship graph — read-only, any distribution.**

!!! warning "Public Preview"
    Mantis is functional and strictly read-only — safe to point at a real
    cluster — but its login flow, UI, and interfaces are still evolving
    ahead of a stable release. See [Public Preview notes](security.md#public-preview-notes)
    for exactly what that does and doesn't mean.

Mantis is a **read-only Kubernetes context and investigation tool**. Point
it at a cluster and it draws the relationships between your resources as a
live, interactive graph — Pods, the Deployments that own them, the Services
that route to them, the ConfigMaps and Secrets they mount, the Nodes they
run on, and more — instead of leaving you to reconstruct that picture
yourself from `kubectl get` and `describe` output.

It answers the questions Kubernetes' raw metadata makes you assemble by hand:

- Why does this Pod exist, and what created it?
- Which Pods does this Service actually route to?
- This Pod is stuck `Pending` — what is it waiting on?
- This Ingress returns a 5xx — does the Service it points at even exist?

Every resource is a node, every relationship is a typed, directional edge,
and namespaces are drawn as visual regions on the canvas. A reference that
points at something Mantis checked and could not find is drawn as a
**dashed, "not found"** edge — Mantis distinguishes what it verified from
what it merely read off a spec field, and never guesses.

Mantis is not a replacement for `kubectl`, k9s, a metrics stack, or a
GitOps tool — it does one job (explain how what's running relates to what
else is running) and stays out of the way of the tools that already do the
others.

## Quick links

<div class="grid cards" markdown>

- :material-rocket-launch:{ .lg .middle } **Getting Started**

    ---

    Install Mantis with Helm and see your first graph in a few minutes.

    [:octicons-arrow-right-24: Get started](getting-started.md)

- :material-graph-outline:{ .lg .middle } **Core Concepts**

    ---

    What a "relationship" means in Mantis, and how it decides what's real.

    [:octicons-arrow-right-24: Core concepts](core-concepts.md)

- :material-shield-check-outline:{ .lg .middle } **Security**

    ---

    The read-only guarantee, Secret handling, and what Public Preview means.

    [:octicons-arrow-right-24: Security](security.md)

- :material-chat-outline:{ .lg .middle } **Community**

    ---

    Join the Discord, open an issue, or just tell us what's confusing.

    [:octicons-arrow-right-24: Contributing](contributing.md)

</div>

## What makes it different

Mantis isn't trying to replace the tools you already use — it solves a
different problem.

| Tool | Its job | Mantis's job |
|---|---|---|
| `kubectl` | Query and mutate resources | Explain how resources relate |
| k9s | Interactive cluster navigation | Context and investigation |
| Prometheus / Grafana | Metrics and dashboards | Relationships, not metrics |
| ArgoCD | "Does live match Git?" (GitOps sync) | "What's the story around this resource?" |

Mantis works on **any** resource whether or not it was deployed through a
pipeline, is strictly **read-only** — it never mutates your cluster — and
runs on **any** Kubernetes distribution (minikube, kind, kubeadm, RKE2,
EKS, GKE, AKS, …) because it talks to the standard Kubernetes API, never to
a specific distro.

## Source and license

Mantis is open source under the [Apache License 2.0](https://github.com/continuumx1/mantis/blob/main/LICENSE).
Source: [github.com/continuumx1/mantis](https://github.com/continuumx1/mantis).
Built and maintained by [ContinuumX1 Technologies](https://continuumx1.com).
