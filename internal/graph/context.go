package graph

import (
	"context"
	"sort"

	"k8s.io/client-go/kubernetes"
)

// Node is a resource participating in the graph together with what Mantis has
// verified about it. A Node exists in a Context only when its existence was
// actually checked, so a Node's presence means "verified" and Resolved reports
// the outcome.
//
// Attributes are compact, pre-formatted display details (e.g. a Pod's health or
// a PVC's size and reclaim policy) that a whole-graph renderer shows inline.
// Hidden marks a node that exists and participates in the graph — so references
// to it are still resolved correctly — but that the map renderer should not list
// on its own (used to suppress system-managed noise unless the caller asks for
// it).
type Node struct {
	Ref        ResourceRef
	Resolved   bool
	Attributes []string
	Hidden     bool
}

// Context is the structured investigation result for a subject resource: the
// relationships discovered around it and the verified state of the nodes those
// relationships point at.
//
// Context models the graph only. Intrinsic properties of the subject (a Pod's
// phase, a Service's selector) stay on the typed object the caller already
// holds; the renderer combines the two. As more is understood about resources
// (configuration, health, change history — see the project roadmap), those
// concerns attach here, at which point Context earns a package of its own.
type Context struct {
	Subject   ResourceRef
	Relations []Relation
	nodes     map[ResourceRef]Node
}

// New assembles a Context from an already-verified existence map. It performs no
// I/O, which keeps it usable directly from tests; Build is the cluster-backed
// entry point.
func New(subject ResourceRef, relations []Relation, existence map[ResourceRef]bool) *Context {
	nodes := make([]Node, 0, len(existence))
	for ref, resolved := range existence {
		nodes = append(nodes, Node{Ref: ref, Resolved: resolved})
	}
	return NewFromNodes(subject, relations, nodes)
}

// NewFromNodes assembles a Context from fully-formed nodes, letting a builder
// attach attributes and hidden flags. It performs no I/O.
func NewFromNodes(subject ResourceRef, relations []Relation, nodes []Node) *Context {
	byRef := make(map[ResourceRef]Node, len(nodes))
	for _, n := range nodes {
		byRef[n.Ref] = n
	}
	return &Context{
		Subject:   subject,
		Relations: relations,
		nodes:     byRef,
	}
}

// Build assembles a Context for a subject and its resolved relations, verifying
// the existence of every target node against the cluster.
func Build(
	ctx context.Context,
	clientset kubernetes.Interface,
	subject ResourceRef,
	relations []Relation,
) (*Context, error) {
	existence, err := VerifyExistence(ctx, clientset, TargetRefs(relations))
	if err != nil {
		return nil, err
	}
	return New(subject, relations, existence), nil
}

// From returns the relations of the given type originating at from.
func (c *Context) From(from ResourceRef, t RelationType) []Relation {
	var out []Relation
	for _, r := range c.Relations {
		if r.From == from && r.Type == t {
			out = append(out, r)
		}
	}
	return out
}

// Into returns the relations of the given type pointing at to. It is the reverse
// of From, enabling queries such as "which Services select this Pod?".
func (c *Context) Into(to ResourceRef, t RelationType) []Relation {
	var out []Relation
	for _, r := range c.Relations {
		if r.To == to && r.Type == t {
			out = append(out, r)
		}
	}
	return out
}

// Existence reports what is known about ref. checked is false for refs whose
// existence was never verified, in which case resolved is meaningless and the
// caller must not treat the resource as missing.
func (c *Context) Existence(ref ResourceRef) (resolved bool, checked bool) {
	n, ok := c.nodes[ref]
	return n.Resolved, ok
}

// Attributes returns the compact display details recorded for ref, if any.
func (c *Context) Attributes(ref ResourceRef) []string {
	return c.nodes[ref].Attributes
}

// Nodes returns every node in the Context, sorted by kind then name. It lets a
// whole-graph renderer enumerate resources (including those that are only ever
// a relationship source, such as a Deployment nothing points at).
func (c *Context) Nodes() []Node {
	out := make([]Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref.Kind != out[j].Ref.Kind {
			return out[i].Ref.Kind < out[j].Ref.Kind
		}
		return out[i].Ref.Name < out[j].Ref.Name
	})
	return out
}
