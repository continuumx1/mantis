package graph

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// BuildClusterGraph assembles the relationship graph for an entire cluster by
// listing every namespace the caller can see and running BuildNamespaceGraph for
// each, then merging the results into one Context. Cluster-scoped resources
// (Nodes, PersistentVolumes) are listed inside each namespace pass, so the merge
// deduplicates them by reference: a resolved node always wins over an unresolved
// one, which also lets a reference that dangles in one namespace resolve against
// a real resource in another.
//
// The merge keeps the whole-cluster map honest without special-casing: every
// listed resource is still a node, and an edge target is unresolved only when it
// exists in no namespace at all. Kinds that RBAC forbids listing are collected
// and returned deduplicated so the caller can report reduced coverage.
//
// It re-lists cluster-scoped resources once per namespace, which is wasteful on
// clusters with very many namespaces; that is a deliberate simplicity trade for
// now and can become a single up-front list later without changing the shape.
func BuildClusterGraph(
	ctx context.Context,
	clientset kubernetes.Interface,
	dyn dynamic.Interface,
	showAll bool,
) (*Context, []string, error) {
	nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("list namespaces: %w", err)
	}

	merged := map[ResourceRef]Node{}
	var relations []Relation
	skippedSet := map[string]struct{}{}

	for i := range nsList.Items {
		namespace := nsList.Items[i].Name

		nsCtx, skipped, err := BuildNamespaceGraph(ctx, clientset, dyn, namespace, showAll)
		if err != nil {
			return nil, nil, err
		}
		for _, kind := range skipped {
			skippedSet[kind] = struct{}{}
		}

		relations = append(relations, nsCtx.Relations...)
		for _, n := range nsCtx.Nodes() {
			mergeNode(merged, n)
		}
	}

	// Karpenter NodePools are cluster-scoped, so list them once here (best-effort
	// via the dynamic client) rather than per namespace.
	for r, a := range collectNodePools(ctx, dyn) {
		mergeNode(merged, Node{Ref: r, Resolved: true, Attributes: a})
	}

	nodes := make([]Node, 0, len(merged))
	for _, n := range merged {
		nodes = append(nodes, n)
	}

	skipped := make([]string, 0, len(skippedSet))
	for kind := range skippedSet {
		skipped = append(skipped, kind)
	}
	sort.Strings(skipped)

	subject := ResourceRef{Kind: "Cluster"}
	return NewFromNodes(subject, relations, nodes), skipped, nil
}

// mergeNode folds a namespace-scoped node into the cluster-wide set. A resolved
// node supersedes an unresolved one for the same reference; when both agree on
// existence, the one carrying attributes is preferred so display detail is not
// lost during the merge.
func mergeNode(merged map[ResourceRef]Node, n Node) {
	existing, ok := merged[n.Ref]
	if !ok {
		merged[n.Ref] = n
		return
	}
	if n.Resolved && !existing.Resolved {
		merged[n.Ref] = n
		return
	}
	if n.Resolved == existing.Resolved && len(existing.Attributes) == 0 && len(n.Attributes) > 0 {
		merged[n.Ref] = n
	}
}
