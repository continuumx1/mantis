package graph

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

// Node is a resource participating in the graph together with what KNW has
// verified about it. A Node exists in a Context only when its existence was
// actually checked, so a Node's presence means "verified" and Resolved reports
// the outcome.
type Node struct {
	Ref      ResourceRef
	Resolved bool
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
	nodes := make(map[ResourceRef]Node, len(existence))
	for ref, resolved := range existence {
		nodes[ref] = Node{Ref: ref, Resolved: resolved}
	}
	return &Context{
		Subject:   subject,
		Relations: relations,
		nodes:     nodes,
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

// Existence reports what is known about ref. checked is false for refs whose
// existence was never verified, in which case resolved is meaningless and the
// caller must not treat the resource as missing.
func (c *Context) Existence(ref ResourceRef) (resolved bool, checked bool) {
	n, ok := c.nodes[ref]
	return n.Resolved, ok
}
