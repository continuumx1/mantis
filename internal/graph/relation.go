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

	// Scales means the source autoscaler (HPA, VPA) drives the replica count or
	// resource requests of the target workload, as declared by its
	// scaleTargetRef / targetRef.
	Scales RelationType = "scales"

	// UsesServiceAccount means the source Pod runs as the target
	// ServiceAccount (spec.serviceAccountName).
	UsesServiceAccount RelationType = "uses-service-account"

	// UsesStorageClass means the source PersistentVolumeClaim or
	// PersistentVolume names the target StorageClass (spec.storageClassName).
	UsesStorageClass RelationType = "storage-class"

	// BindsRole means the source RoleBinding or ClusterRoleBinding grants the
	// target Role or ClusterRole (roleRef).
	BindsRole RelationType = "binds-role"

	// GrantsTo means the source RoleBinding or ClusterRoleBinding grants its
	// role to the target ServiceAccount (subjects[]). User and Group subjects
	// are not Kubernetes resources and therefore never become edges.
	GrantsTo RelationType = "grants-to"

	// AppliesTo means the source NetworkPolicy's spec.podSelector selects the
	// target Pod. A NetworkPolicy only ever selects Pods in its own namespace.
	AppliesTo RelationType = "applies-to"
)

// Category groups relation types into the families a renderer styles and
// filters by. Every RelationType maps to exactly one category.
type Category string

const (
	CategoryOwnership     Category = "ownership"
	CategoryScheduling    Category = "scheduling"
	CategoryNetworking    Category = "networking"
	CategoryConfiguration Category = "configuration"
	CategoryStorage       Category = "storage"
	CategorySecurity      Category = "security"
	CategoryAutoscaling   Category = "autoscaling"
)

// Basis says what kind of evidence a relationship rests on. It exists so a
// renderer never presents, say, a label-selector match as observed traffic:
// Mantis reads the Kubernetes API only and has no network telemetry, so no
// edge it produces today is "traffic" — the strongest runtime evidence it has
// is the control plane's own EndpointSlice record.
type Basis string

const (
	// BasisOwnership is an explicit metadata.ownerReferences entry.
	BasisOwnership Basis = "ownership"
	// BasisReference is a name declared in one object's spec that points at
	// another (env/volume refs, spec.nodeName, roleRef, scaleTargetRef, ...).
	BasisReference Basis = "reference"
	// BasisSelector is a label-selector match computed by Mantis from the
	// selector and the target's labels, within the selector's namespace.
	BasisSelector Basis = "selector"
	// BasisEndpoint is the control plane's EndpointSlice record of which Pods
	// back a Service. It is runtime state, but it is not observed traffic.
	BasisEndpoint Basis = "endpoint"
	// BasisInferred is anything deduced rather than read. Nothing produces it
	// yet; it is reserved so a future inferred edge is never mistaken for fact.
	BasisInferred Basis = "inferred"
)

// relationMeta is the static description of each relation type.
var relationMeta = map[RelationType]struct {
	category Category
	basis    Basis
	evidence string
}{
	ControlledBy:       {CategoryOwnership, BasisOwnership, "metadata.ownerReferences"},
	RunsOn:             {CategoryScheduling, BasisReference, "spec.nodeName"},
	Selects:            {CategoryNetworking, BasisSelector, "spec.selector"},
	Serves:             {CategoryNetworking, BasisEndpoint, "EndpointSlice endpoints"},
	RoutesTo:           {CategoryNetworking, BasisReference, "spec.rules[].http.paths[].backend.service"},
	References:         {CategoryConfiguration, BasisReference, "env / envFrom"},
	Mounts:             {CategoryConfiguration, BasisReference, "spec.volumes"},
	UsesServiceAccount: {CategoryConfiguration, BasisReference, "spec.serviceAccountName"},
	Claims:             {CategoryStorage, BasisReference, "spec.volumes[].persistentVolumeClaim"},
	BoundTo:            {CategoryStorage, BasisReference, "spec.volumeName"},
	UsesStorageClass:   {CategoryStorage, BasisReference, "spec.storageClassName"},
	BindsRole:          {CategorySecurity, BasisReference, "roleRef"},
	GrantsTo:           {CategorySecurity, BasisReference, "subjects[]"},
	AppliesTo:          {CategoryNetworking, BasisSelector, "spec.podSelector"},
	Scales:             {CategoryAutoscaling, BasisReference, "spec.scaleTargetRef"},
}

// Category reports the family a relation type belongs to. An unknown type
// (a future type not yet described here) falls back to configuration rather
// than panicking — the edge is still drawn, just conservatively styled.
func (t RelationType) Category() Category {
	if m, ok := relationMeta[t]; ok {
		return m.category
	}
	return CategoryConfiguration
}

// Basis reports what kind of evidence a relation type rests on.
func (t RelationType) Basis() Basis {
	if m, ok := relationMeta[t]; ok {
		return m.basis
	}
	return BasisReference
}

// Evidence names the Kubernetes API field a relation type is read from.
func (t RelationType) Evidence() string {
	return relationMeta[t].evidence
}

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

// Key is a stable identity for the edge, used to deduplicate relations that
// are discovered more than once (for example, the same cluster-scoped edge
// reported while walking several namespaces).
func (r Relation) Key() string {
	return r.From.String() + "|" + string(r.Type) + "|" + r.To.String()
}

// Basis reports the evidence class of this specific relation: an Inferred
// relation is always BasisInferred regardless of its type.
func (r Relation) Basis() Basis {
	if r.IsInferred() {
		return BasisInferred
	}
	return r.Type.Basis()
}

// IsInferred reports whether the relationship was deduced rather than read from
// the API, in which case a renderer must mark it as not a verified fact.
func (r Relation) IsInferred() bool {
	return r.Certainty == Inferred
}
