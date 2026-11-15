# Core Concepts

## Resources, relationships, and groups

Mantis represents your cluster as a graph:

- **Nodes** are Kubernetes resources — Pods, Services, Deployments, and
  everything else Mantis maps (see [Resource kinds](#resource-kinds) below).
- **Edges** are typed, directional relationships between them — see
  [Relationship types](#relationship-types).
- **Groups** organise the picture: in the default view each namespace is a
  group, and cluster-scoped resources (Nodes, PersistentVolumes,
  StorageClasses, ClusterRoles, NodePools) get their own "cluster
  infrastructure" group rather than being forced into a namespace. The
  Nodes and Workloads views group by node and by application instead.

## Verified, not guessed

A key principle running through Mantis: it distinguishes **facts it read**
from the Kubernetes API from **references it could not verify**.

If a Service's selector matches no Pods, or a PVC's `volumeName` points at
a PersistentVolume that doesn't exist, Mantis doesn't hide that or silently
draw a normal-looking edge — it checked, found nothing, and shows a
**dashed, "not found"** edge instead. This is deliberate: a dangling
reference is exactly the kind of thing worth surfacing, not smoothing over.

## Resource kinds

Core, built-in kinds:

- **Workloads:** `Pod`, `Deployment`, `ReplicaSet`, `StatefulSet`,
  `DaemonSet`, `Job`, `CronJob`
- **Networking:** `Service`, `EndpointSlice` (used to verify routing),
  `Ingress`, `NetworkPolicy`
- **Configuration:** `ConfigMap`, `Secret` (metadata only — see
  [Security](security.md)), `ResourceQuota`, `LimitRange`
- **Storage:** `PersistentVolumeClaim`, `PersistentVolume`, `StorageClass`
- **Security:** `ServiceAccount`, `Role`, `RoleBinding`, `ClusterRole`,
  `ClusterRoleBinding`
- **Cluster and scaling:** `Node`, `Namespace`, `HorizontalPodAutoscaler`

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
| `uses-service-account` | A Pod runs as a ServiceAccount | `pod.spec.serviceAccountName` |
| `storage-class` | A PVC or PV uses a StorageClass | `spec.storageClassName` |
| `binds-role` | A (Cluster)RoleBinding grants a Role or ClusterRole | `roleRef` |
| `grants-to` | A (Cluster)RoleBinding grants its role to a ServiceAccount | `subjects` |
| `applies-to` | A NetworkPolicy selects a Pod | `spec.podSelector` |

Each relationship belongs to a category — ownership, scheduling,
networking, configuration, storage, security or autoscaling — which the
Topology page uses for line styles and filters, and carries its evidence:
whether Mantis verified it against the cluster (an EndpointSlice, a
resolved reference) or read it from a spec field.

Relationships are named for their **Kubernetes semantics**, not for
convenience — `runs-on` means exactly what `kubectl get pod -o wide`'s NODE
column means, nothing more inferred.

## How Mantis stays current

The engine lists each kind once, then follows the cluster with Kubernetes
watches and rebuilds the graph from its cache when something changes,
usually within a second. The browser asks for what changed since its last
revision every 10 seconds by default (configurable in Settings, or
manual), so what you see is at most one refresh interval old. If the API
server refuses watches, the engine falls back to re-listing on an interval
on its own. See
[the graph architecture](graph-architecture.md#keeping-the-graph-current-watch-not-poll).
