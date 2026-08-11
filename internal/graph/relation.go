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
)

// Relation is a single directed edge in the resource graph: From is related to
// To with the given semantic Type.
type Relation struct {
	From ResourceRef
	Type RelationType
	To   ResourceRef
}
