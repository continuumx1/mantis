package graph

// RelationType names the semantic meaning of a relationship between two
// resources. Names must reflect actual Kubernetes API semantics rather than
// implementation convenience.
type RelationType string

const (
	// ControlledBy means the target controls the source, as expressed by an
	// ownerReference with controller == true (Pod -> ReplicaSet -> Deployment).
	ControlledBy RelationType = "controlled-by"

	// RunsOn means the source Pod is scheduled onto the target Node.
	RunsOn RelationType = "runs-on"

	// Selects means the source Service selects the target Pod via its label
	// selector (spec.selector matching the Pod's labels).
	Selects RelationType = "selects"

	// RoutesTo means the source Ingress routes traffic to the target Service,
	// as declared by an ingress backend (default backend or a rule path).
	RoutesTo RelationType = "routes-to"

	// References means the source Pod consumes the target ConfigMap or Secret
	// through the environment (env / envFrom).
	References RelationType = "references"

	// Mounts means the source Pod mounts the target ConfigMap or Secret as a
	// volume.
	Mounts RelationType = "mounts"

	// Claims means the source Pod claims the target PersistentVolumeClaim
	// through a volume.
	Claims RelationType = "claims"

	// BoundTo means the source PersistentVolumeClaim is bound to the target
	// PersistentVolume.
	BoundTo RelationType = "bound-to"
)

// Relation is a single directed edge in the resource graph: From is related to
// To with the given semantic Type.
type Relation struct {
	From ResourceRef
	Type RelationType
	To   ResourceRef
}
