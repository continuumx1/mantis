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

	// Serves means the source Service actually backs the target Pod, as recorded
	// in its EndpointSlices — the real traffic path, which can differ from the
	// selector match (e.g. a matched Pod that is not a ready endpoint).
	Serves RelationType = "serves"

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

// Certainty records how much Mantis trusts a relationship. Every edge today is
// Observed — derived directly from a Kubernetes API field — so the zero value
// means Observed. Inferred is reserved for future edges deduced rather than
// read, which must always be rendered as such and never presented as fact.
type Certainty string

const (
	// Observed edges are read straight from the API (the default).
	Observed Certainty = ""
	// Inferred edges are deduced and must be visibly distinguished.
	Inferred Certainty = "inferred"
)

// Relation is a single directed edge in the resource graph: From is related to
// To with the given semantic Type. Certainty defaults to Observed.
type Relation struct {
	From      ResourceRef
	Type      RelationType
	To        ResourceRef
	Certainty Certainty
}

// IsInferred reports whether the relationship was deduced rather than read from
// the API, in which case a renderer must mark it as not a verified fact.
func (r Relation) IsInferred() bool {
	return r.Certainty == Inferred
}
