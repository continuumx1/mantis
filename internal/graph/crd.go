package graph

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// This file reads optional Custom Resources — VerticalPodAutoscaler and
// Karpenter's node-autoscaling CRDs — through the dynamic client. None of them
// are guaranteed to be installed, so every collector treats a failed List (the
// CRD is absent, or RBAC forbids it) as "not present" and returns nothing rather
// than failing the whole graph. A nil dynamic client (e.g. in tests) also yields
// nothing. This keeps CRD awareness purely additive.

var (
	// vpaGVR is the VerticalPodAutoscaler custom resource.
	vpaGVR = schema.GroupVersionResource{Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers"}
	// karpenterNodePoolGVR is Karpenter's cluster-scoped NodePool. Karpenter
	// graduated its API to v1; older clusters expose v1beta1, tried as a fallback.
	karpenterNodePoolGVR     = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"}
	karpenterNodePoolGVRBeta = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1beta1", Resource: "nodepools"}
)

// collectVPAs lists VerticalPodAutoscalers in a namespace and returns their node
// attributes and the scales edges to the workloads they tune. It is best-effort:
// a missing CRD or a nil client yields no results and no error.
func collectVPAs(ctx context.Context, dyn dynamic.Interface, namespace string) (map[ResourceRef][]string, []Relation) {
	if dyn == nil {
		return nil, nil
	}
	list, err := dyn.Resource(vpaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil // CRD absent or forbidden → skip silently.
	}

	attrs := map[ResourceRef][]string{}
	var relations []Relation
	for i := range list.Items {
		item := &list.Items[i]
		vpaRef := ResourceRef{Kind: "VerticalPodAutoscaler", Name: item.GetName(), Namespace: namespace}
		attrs[vpaRef] = vpaAttributes(item)
		if kind, name, ok := targetRef(item, "spec", "targetRef"); ok {
			relations = append(relations, Relation{
				From: vpaRef,
				Type: Scales,
				To:   ResourceRef{Kind: kind, Name: name, Namespace: namespace},
			})
		}
	}
	return attrs, relations
}

// vpaAttributes summarizes a VPA: its update mode (Off/Initial/Auto) and target.
func vpaAttributes(vpa *unstructured.Unstructured) []string {
	var attrs []string
	if mode, ok, _ := unstructured.NestedString(vpa.Object, "spec", "updatePolicy", "updateMode"); ok && mode != "" {
		attrs = append(attrs, "updateMode: "+mode)
	} else {
		attrs = append(attrs, "updateMode: Auto")
	}
	if kind, name, ok := targetRef(vpa, "spec", "targetRef"); ok {
		attrs = append(attrs, fmt.Sprintf("target: %s/%s", kind, name))
	}
	return attrs
}

// collectNodePools lists Karpenter NodePools (cluster-scoped) and returns them as
// node attributes. Their mere presence is what answers "is there a cluster node
// autoscaler?"; nodeAutoscalerKind reports that separately for the header.
func collectNodePools(ctx context.Context, dyn dynamic.Interface) map[ResourceRef][]string {
	if dyn == nil {
		return nil
	}
	list, err := dyn.Resource(karpenterNodePoolGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if list, err = dyn.Resource(karpenterNodePoolGVRBeta).List(ctx, metav1.ListOptions{}); err != nil {
			return nil
		}
	}

	attrs := map[ResourceRef][]string{}
	for i := range list.Items {
		item := &list.Items[i]
		ref := ResourceRef{Kind: "NodePool", Name: item.GetName()}
		attrs[ref] = nodePoolAttributes(item)
	}
	return attrs
}

// nodePoolAttributes summarizes a Karpenter NodePool: how many nodes/resources it
// currently manages, if the status reports them.
func nodePoolAttributes(np *unstructured.Unstructured) []string {
	attrs := []string{"provisioner: karpenter"}
	if nodes, ok, _ := unstructured.NestedInt64(np.Object, "status", "resources", "nodes"); ok {
		attrs = append(attrs, fmt.Sprintf("nodes: %d", nodes))
	}
	if cpu, ok, _ := unstructured.NestedString(np.Object, "status", "resources", "cpu"); ok && cpu != "" {
		attrs = append(attrs, "cpu: "+cpu)
	}
	return attrs
}

// DetectNodeAutoscaler reports which cluster-level node autoscaler is present, if
// any: "Karpenter" when NodePools exist, or "cluster-autoscaler" when the classic
// autoscaler's status ConfigMap is found in kube-system. It returns "" when
// neither is detected. Every probe is best-effort — absence, RBAC denial, or a
// nil client all read as "not present".
func DetectNodeAutoscaler(ctx context.Context, dyn dynamic.Interface, clientset kubernetes.Interface) string {
	if dyn != nil {
		if list, err := dyn.Resource(karpenterNodePoolGVR).List(ctx, metav1.ListOptions{}); err == nil && len(list.Items) > 0 {
			return "Karpenter"
		}
		if list, err := dyn.Resource(karpenterNodePoolGVRBeta).List(ctx, metav1.ListOptions{}); err == nil && len(list.Items) > 0 {
			return "Karpenter"
		}
	}
	if clientset != nil {
		// The classic cluster-autoscaler leases/publishes a status ConfigMap.
		if _, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "cluster-autoscaler-status", metav1.GetOptions{}); err == nil {
			return "cluster-autoscaler"
		}
	}
	return ""
}

// targetRef reads a {kind,name} scale target from an unstructured spec path.
func targetRef(obj *unstructured.Unstructured, fields ...string) (kind, name string, ok bool) {
	kind, kok, _ := unstructured.NestedString(obj.Object, append(fields, "kind")...)
	name, nok, _ := unstructured.NestedString(obj.Object, append(fields, "name")...)
	if !kok || !nok || kind == "" || name == "" {
		return "", "", false
	}
	return kind, name, true
}
