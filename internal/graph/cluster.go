package graph

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// ClusterSnapshot is one point-in-time render of the whole-cluster merge as
// BuildClusterGraphProgressive assembles it, namespace by namespace. Context
// and Skipped are already a complete, independent, safe-to-keep copy of
// everything merged so far — not a view onto state the walk keeps mutating —
// so a caller can publish one straight into a cache read concurrently by
// other goroutines.
type ClusterSnapshot struct {
	Context         *Context
	Skipped         []string
	NamespacesDone  int
	NamespacesTotal int
	// Complete is true only on the final report of a pass (after every
	// namespace and the cluster-scoped extras), false on every partial one.
	Complete bool
}

// BuildClusterGraph assembles the relationship graph for an entire cluster in
// one blocking call: every namespace the caller can see, merged into one
// Context. It is a thin wrapper around BuildClusterGraphProgressive that
// keeps only the final report — for a caller (tests included) that just
// wants one complete result and does not care how it got there.
func BuildClusterGraph(
	ctx context.Context,
	clientset kubernetes.Interface,
	dyn dynamic.Interface,
	showAll bool,
) (*Context, []string, error) {
	var final ClusterSnapshot
	err := BuildClusterGraphProgressive(ctx, clientset, dyn, showAll, func(snap ClusterSnapshot) {
		final = snap
	})
	if err != nil {
		return nil, nil, err
	}
	return final.Context, final.Skipped, nil
}

// BuildClusterGraphProgressive lists every namespace the caller can see, then
// builds and merges each one's graph exactly as BuildClusterGraph always has
// — same BuildNamespaceGraph call, same merge-by-reference dedup for
// cluster-scoped resources (Nodes, PersistentVolumes) so a resolved node
// always wins over an unresolved one, and a reference that dangles in one
// namespace can still resolve against a real resource found in another —
// except report is called with the merged graph *as it stands so far* after
// every namespace, not just once at the end.
//
// This is what turns "read the whole cluster before anyone gets an answer"
// into "the first namespace's worth of resources reaches the caller as soon
// as it's ready, and every namespace after that fills the picture in a
// little more" — a large cluster's total build time no longer has to fit
// inside one HTTP request's timeout, because no single request is waiting on
// all of it.
//
// It re-lists cluster-scoped resources once per namespace, which is wasteful
// on clusters with very many namespaces; that is a deliberate simplicity
// trade carried over from BuildClusterGraph's original shape, not something
// this change introduces.
func BuildClusterGraphProgressive(
	ctx context.Context,
	clientset kubernetes.Interface,
	dyn dynamic.Interface,
	showAll bool,
	report func(ClusterSnapshot),
) error {
	nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	merged := map[ResourceRef]Node{}
	var relations []Relation
	skippedSet := map[string]struct{}{}
	total := len(nsList.Items)

	// publish snapshots the current merge state into an independent
	// ClusterSnapshot — fresh slices, not the loop's own backing arrays/map —
	// so report's caller can hold onto (or hand to another goroutine) what it
	// receives without it changing underneath them as the walk continues.
	publish := func(done int, complete bool) {
		nodes := make([]Node, 0, len(merged))
		for _, n := range merged {
			nodes = append(nodes, n)
		}
		skipped := make([]string, 0, len(skippedSet))
		for kind := range skippedSet {
			skipped = append(skipped, kind)
		}
		sort.Strings(skipped)
		relCopy := make([]Relation, len(relations))
		copy(relCopy, relations)

		subject := ResourceRef{Kind: "Cluster"}
		report(ClusterSnapshot{
			Context:         NewFromNodes(subject, relCopy, nodes),
			Skipped:         skipped,
			NamespacesDone:  done,
			NamespacesTotal: total,
			Complete:        complete,
		})
	}

	for i := range nsList.Items {
		if err := ctx.Err(); err != nil {
			return err
		}
		namespace := nsList.Items[i].Name

		nsCtx, skipped, err := BuildNamespaceGraph(ctx, clientset, dyn, namespace, showAll)
		if err != nil {
			return err
		}
		for _, kind := range skipped {
			skippedSet[kind] = struct{}{}
		}

		relations = append(relations, nsCtx.Relations...)
		for _, n := range nsCtx.Nodes() {
			mergeNode(merged, n)
		}
		publish(i+1, false)
	}

	// Karpenter NodePools are cluster-scoped, so list them once here (best-effort
	// via the dynamic client) rather than per namespace.
	for r, a := range collectNodePools(ctx, dyn) {
		mergeNode(merged, Node{Ref: r, Resolved: true, Attributes: a})
	}
	publish(total, true)

	return nil
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
