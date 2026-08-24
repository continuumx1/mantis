# Core Concepts

## Resources, relationships, and regions

Mantis represents your cluster as a graph:

- **Nodes** are Kubernetes resources — Pods, Services, Deployments, and
  everything else Mantis maps (see [Resource kinds](#resource-kinds) below).
- **Edges** are typed, directional relationships between them — see
  [Relationship types](#relationship-types).
- **Namespaces** are drawn as soft visual regions on the canvas, not hard
  containers — they're for orientation, and you can still drag any node
  anywhere. Cluster-scoped resources (Nodes, NodePools) get their own
  region rather than being forced into a namespace they don't belong to.

## Verified, not guessed

A key principle running through Mantis: it distinguishes **facts it read**
from the Kubernetes API from **references it could not verify**.

If a Service's selector matches no Pods, or a PVC's `volumeName` points at
a PersistentVolume that doesn't exist, Mantis doesn't hide that or silently
draw a normal-looking edge — it checked, found nothing, and shows a
**dashed, "not found"** edge instead. This is deliberate: a dangling
reference is exactly the kind of thing worth surfacing, not smoothing over.

## Resource kinds

Core, built-in kinds: `Pod`, `Node`, `Service`, `Ingress`, `ConfigMap`,
`Secret` (metadata only — see [Security](security.md)),
`PersistentVolumeClaim`, `PersistentVolume`, `ResourceQuota`, `LimitRange`,
`Deployment`, `ReplicaSet`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`,
`HorizontalPodAutoscaler`.

Optional, best-effort custom resources (included automatically if the CRD
is installed and RBAC allows reading it; silently omitted otherwise, never
an error): `VerticalPodAutoscaler`, Karpenter `NodePool`. Mantis also
detects — and notes in the header — whether a cluster-level node
autoscaler (Karpenter or the classic `cluster-autoscaler`) appears to be
running, without requiring either to be present.

## Relationship types

| Relationship | Meaning | Derived from |
|---|---|---|
| `controlled-by` | A resource is controlled by its owner | `ownerReferences` |
| `runs-on` | A Pod is scheduled onto a Node | `pod.spec.nodeName` |
| `selects` | A Service selects Pods | `service.spec.selector` |
| `serves` | A Service actually backs a Pod | EndpointSlices |
| `routes-to` | An Ingress routes to a Service | ingress backends |
| `references` | A Pod consumes config via the environment | `env` / `envFrom` |
| `mounts` | A Pod mounts config as a volume | `volumes` |
| `claims` | A Pod claims a PersistentVolumeClaim | `volumes[].persistentVolumeClaim` |
| `bound-to` | A PVC is bound to a PersistentVolume | `pvc.spec.volumeName` |
| `scales` | An autoscaler (HPA/VPA) drives a workload's replica count or resources | HPA/VPA `scaleTargetRef` / `targetRef` |

Relationships are named for their **Kubernetes semantics**, not for
convenience — `runs-on` means exactly what `kubectl get pod -o wide`'s NODE
column means, nothing more inferred.

## A snapshot, not a stream

Mantis builds the graph fresh on every sync — it polls, on an interval you
control (as fast as every 3 seconds, or manual), rather than holding a
persistent `watch` subscription. What you see is accurate as of the last
sync, not necessarily this exact instant. See
[Known limitations](troubleshooting.md#known-limitations) for what that
does and doesn't affect.
