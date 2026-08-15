package graph

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// BuildNamespaceGraph lists the resources Mantis understands in a namespace and
// assembles the full relationship graph for them as a single Context whose
// subject is the namespace. It also returns the kinds it was not permitted to
// list, so the caller can report them.
//
// Existence is decided by membership: every listed resource is a node, and an
// edge target is "resolved" exactly when it is one of those nodes. This needs
// one List per kind (no per-reference Gets), so it scales to a whole namespace,
// and it still honestly flags a reference to something that is not there.
// Cluster-scoped Nodes and PersistentVolumes are listed too so that runs-on and
// bound-to targets resolve.
//
// When showAll is false, cluster-managed noise (the kube-root-ca.crt ConfigMap
// and service-account/Helm Secrets) is kept in the graph — so references to it
// still resolve — but marked Hidden so the renderer omits it. A List that is
// Forbidden by RBAC is skipped and recorded rather than failing the whole map.
func BuildNamespaceGraph(
	ctx context.Context,
	clientset kubernetes.Interface,
	dyn dynamic.Interface,
	namespace string,
	showAll bool,
) (*Context, []string, error) {
	var relations []Relation
	var skipped []string
	nodeSet := map[ResourceRef]struct{}{}
	attrs := map[ResourceRef][]string{}
	hidden := map[ResourceRef]struct{}{}

	ref := func(kind, name, ns string) ResourceRef {
		r := ResourceRef{Kind: kind, Name: name, Namespace: ns}
		nodeSet[r] = struct{}{}
		return r
	}

	opts := metav1.ListOptions{}

	// forbidden records a Forbidden List error and reports whether it was
	// handled; a non-Forbidden error is returned to the caller.
	forbidden := func(kind string, err error) bool {
		if apierrors.IsForbidden(err) {
			skipped = append(skipped, kind)
			return true
		}
		return false
	}

	// Workloads — ownerReferences on every object reconstruct the controller
	// topology (Deployment/ReplicaSet, StatefulSet, DaemonSet, CronJob/Job).
	if list, err := clientset.AppsV1().Deployments(namespace).List(ctx, opts); err != nil {
		if !forbidden("deployments", err) {
			return nil, nil, fmt.Errorf("list deployments: %w", err)
		}
	} else {
		for i := range list.Items {
			d := &list.Items[i]
			dRef := ref("Deployment", d.Name, namespace)
			attrs[dRef] = deploymentAttributes(d)
			relations = append(relations, ownerEdges(dRef, d.OwnerReferences, namespace)...)
		}
	}

	if list, err := clientset.AppsV1().ReplicaSets(namespace).List(ctx, opts); err != nil {
		if !forbidden("replicasets", err) {
			return nil, nil, fmt.Errorf("list replicasets: %w", err)
		}
	} else {
		for i := range list.Items {
			rs := &list.Items[i]
			rsRef := ref("ReplicaSet", rs.Name, namespace)
			attrs[rsRef] = replicaSetAttributes(rs)
			relations = append(relations, ownerEdges(rsRef, rs.OwnerReferences, namespace)...)
		}
	}

	if list, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, opts); err != nil {
		if !forbidden("statefulsets", err) {
			return nil, nil, fmt.Errorf("list statefulsets: %w", err)
		}
	} else {
		for i := range list.Items {
			sts := &list.Items[i]
			stsRef := ref("StatefulSet", sts.Name, namespace)
			attrs[stsRef] = statefulSetAttributes(sts)
			relations = append(relations, ownerEdges(stsRef, sts.OwnerReferences, namespace)...)
		}
	}

	if list, err := clientset.AppsV1().DaemonSets(namespace).List(ctx, opts); err != nil {
		if !forbidden("daemonsets", err) {
			return nil, nil, fmt.Errorf("list daemonsets: %w", err)
		}
	} else {
		for i := range list.Items {
			ds := &list.Items[i]
			dsRef := ref("DaemonSet", ds.Name, namespace)
			attrs[dsRef] = daemonSetAttributes(ds)
			relations = append(relations, ownerEdges(dsRef, ds.OwnerReferences, namespace)...)
		}
	}

	if list, err := clientset.BatchV1().Jobs(namespace).List(ctx, opts); err != nil {
		if !forbidden("jobs", err) {
			return nil, nil, fmt.Errorf("list jobs: %w", err)
		}
	} else {
		for i := range list.Items {
			j := &list.Items[i]
			jRef := ref("Job", j.Name, namespace)
			attrs[jRef] = jobAttributes(j)
			relations = append(relations, ownerEdges(jRef, j.OwnerReferences, namespace)...)
		}
	}

	if list, err := clientset.BatchV1().CronJobs(namespace).List(ctx, opts); err != nil {
		if !forbidden("cronjobs", err) {
			return nil, nil, fmt.Errorf("list cronjobs: %w", err)
		}
	} else {
		for i := range list.Items {
			cj := &list.Items[i]
			cjRef := ref("CronJob", cj.Name, namespace)
			attrs[cjRef] = cronJobAttributes(cj)
			relations = append(relations, ownerEdges(cjRef, cj.OwnerReferences, namespace)...)
		}
	}

	// Pods carry ownership, scheduling, configuration, storage, and a health note.
	var podItems []corev1.Pod
	if list, err := clientset.CoreV1().Pods(namespace).List(ctx, opts); err != nil {
		if !forbidden("pods", err) {
			return nil, nil, fmt.Errorf("list pods: %w", err)
		}
	} else {
		podItems = list.Items
		for i := range podItems {
			p := &podItems[i]
			podRef := ref("Pod", p.Name, namespace)
			attrs[podRef] = podAttributes(p)
			relations = append(relations, ownerEdges(podRef, p.OwnerReferences, namespace)...)
			if p.Spec.NodeName != "" {
				relations = append(relations, Relation{From: podRef, Type: RunsOn, To: ResourceRef{Kind: "Node", Name: p.Spec.NodeName}})
			}
			relations = append(relations, podConfigurationRelations(podRef, p)...)
			relations = append(relations, podStorageRelations(podRef, p)...)
		}
	}

	// EndpointSlices (listed once) give services their real backends.
	var sliceItems []discoveryv1.EndpointSlice
	if list, err := clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, opts); err != nil {
		if !forbidden("endpointslices", err) {
			return nil, nil, fmt.Errorf("list endpointslices: %w", err)
		}
	} else {
		sliceItems = list.Items
	}

	if list, err := clientset.CoreV1().Services(namespace).List(ctx, opts); err != nil {
		if !forbidden("services", err) {
			return nil, nil, fmt.Errorf("list services: %w", err)
		}
	} else {
		for i := range list.Items {
			s := &list.Items[i]
			svcRef := ref("Service", s.Name, namespace)
			attrs[svcRef] = serviceAttributes(s, sliceItems)
			relations = append(relations, selectsRelations(svcRef, s.Spec.Selector, podItems)...)
			relations = append(relations, servesFromSlices(svcRef, sliceItems)...)
		}
	}

	if list, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, opts); err != nil {
		if !forbidden("ingresses", err) {
			return nil, nil, fmt.Errorf("list ingresses: %w", err)
		}
	} else {
		for i := range list.Items {
			ing := &list.Items[i]
			ref("Ingress", ing.Name, namespace)
			edges, err := ResolveIngress(ctx, clientset, ing)
			if err != nil {
				return nil, nil, err
			}
			relations = append(relations, edges...)
		}
	}

	if list, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, opts); err != nil {
		if !forbidden("configmaps", err) {
			return nil, nil, fmt.Errorf("list configmaps: %w", err)
		}
	} else {
		for i := range list.Items {
			cmRef := ref("ConfigMap", list.Items[i].Name, namespace)
			if !showAll && isSystemConfigMap(list.Items[i].Name) {
				hidden[cmRef] = struct{}{}
			}
		}
	}

	if list, err := clientset.CoreV1().Secrets(namespace).List(ctx, opts); err != nil {
		if !forbidden("secrets", err) {
			return nil, nil, fmt.Errorf("list secrets: %w", err)
		}
	} else {
		for i := range list.Items {
			s := &list.Items[i]
			// The List call itself returns full Secret objects — Kubernetes has no
			// "metadata only" projection for typed List — so the values genuinely
			// pass through this process's memory for a moment. Wiping Data and
			// StringData immediately, before anything else touches this slice,
			// turns "we only ever read Name and Type" from an implicit habit this
			// function happens to follow into a real guarantee: nothing added here
			// or downstream — a future attribute builder, an error message, a
			// debug log — can serialize a value that has already been zeroed out.
			s.Data = nil
			s.StringData = nil
			secretRef := ref("Secret", s.Name, namespace)
			if !showAll && isSystemSecret(s) {
				hidden[secretRef] = struct{}{}
			}
		}
	}

	if list, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, opts); err != nil {
		if !forbidden("persistentvolumeclaims", err) {
			return nil, nil, fmt.Errorf("list persistentvolumeclaims: %w", err)
		}
	} else {
		for i := range list.Items {
			pvc := &list.Items[i]
			pvcRef := ref("PersistentVolumeClaim", pvc.Name, namespace)
			attrs[pvcRef] = pvcAttributes(pvc)
			if pvc.Spec.VolumeName != "" {
				relations = append(relations, Relation{From: pvcRef, Type: BoundTo, To: ResourceRef{Kind: "PersistentVolume", Name: pvc.Spec.VolumeName}})
			}
		}
	}

	// ResourceQuota and LimitRange capture the namespace-level requests/limits
	// governance (how much the namespace may consume, and the defaults applied to
	// pods that omit their own requests/limits).
	if list, err := clientset.CoreV1().ResourceQuotas(namespace).List(ctx, opts); err != nil {
		if !forbidden("resourcequotas", err) {
			return nil, nil, fmt.Errorf("list resourcequotas: %w", err)
		}
	} else {
		for i := range list.Items {
			rq := &list.Items[i]
			attrs[ref("ResourceQuota", rq.Name, namespace)] = resourceQuotaAttributes(rq)
		}
	}

	if list, err := clientset.CoreV1().LimitRanges(namespace).List(ctx, opts); err != nil {
		if !forbidden("limitranges", err) {
			return nil, nil, fmt.Errorf("list limitranges: %w", err)
		}
	} else {
		for i := range list.Items {
			lr := &list.Items[i]
			attrs[ref("LimitRange", lr.Name, namespace)] = limitRangeAttributes(lr)
		}
	}

	// HorizontalPodAutoscalers scale a workload's replica count; the scales edge
	// points from the HPA to the Deployment/StatefulSet it targets.
	if list, err := clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, opts); err != nil {
		if !forbidden("horizontalpodautoscalers", err) {
			return nil, nil, fmt.Errorf("list horizontalpodautoscalers: %w", err)
		}
	} else {
		for i := range list.Items {
			hpa := &list.Items[i]
			hpaRef := ref("HorizontalPodAutoscaler", hpa.Name, namespace)
			attrs[hpaRef] = hpaAttributes(hpa)
			target := hpa.Spec.ScaleTargetRef
			relations = append(relations, Relation{
				From: hpaRef,
				Type: Scales,
				To:   ResourceRef{Kind: target.Kind, Name: target.Name, Namespace: namespace},
			})
		}
	}

	// VerticalPodAutoscalers (a CRD, so read via the dynamic client and skipped
	// when the CRD is not installed) tune a workload's resource requests.
	vpaAttrs, vpaRelations := collectVPAs(ctx, dyn, namespace)
	for r, a := range vpaAttrs {
		nodeSet[r] = struct{}{}
		attrs[r] = a
	}
	relations = append(relations, vpaRelations...)

	// Cluster-scoped resources, listed so runs-on and bound-to targets resolve.
	if list, err := clientset.CoreV1().Nodes().List(ctx, opts); err != nil {
		if !forbidden("nodes", err) {
			return nil, nil, fmt.Errorf("list nodes: %w", err)
		}
	} else {
		for i := range list.Items {
			node := &list.Items[i]
			attrs[ref("Node", node.Name, "")] = nodeAttributes(node)
		}
	}

	if list, err := clientset.CoreV1().PersistentVolumes().List(ctx, opts); err != nil {
		if !forbidden("persistentvolumes", err) {
			return nil, nil, fmt.Errorf("list persistentvolumes: %w", err)
		}
	} else {
		for i := range list.Items {
			pv := &list.Items[i]
			pvRef := ref("PersistentVolume", pv.Name, "")
			attrs[pvRef] = pvAttributes(pv)
		}
	}

	nodes := make([]Node, 0, len(nodeSet))
	for r := range nodeSet {
		_, isHidden := hidden[r]
		nodes = append(nodes, Node{Ref: r, Resolved: true, Attributes: attrs[r], Hidden: isHidden})
	}
	// Dangling targets: referenced but never listed → verified absent.
	danglingSeen := map[ResourceRef]struct{}{}
	for _, rel := range relations {
		if _, ok := nodeSet[rel.To]; ok {
			continue
		}
		if _, ok := danglingSeen[rel.To]; ok {
			continue
		}
		danglingSeen[rel.To] = struct{}{}
		nodes = append(nodes, Node{Ref: rel.To, Resolved: false})
	}

	subject := ResourceRef{Kind: "Namespace", Name: namespace}
	return NewFromNodes(subject, relations, nodes), skipped, nil
}
