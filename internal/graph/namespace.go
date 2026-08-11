package graph

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// BuildNamespaceGraph lists the resources KNW understands in a namespace and
// assembles the full relationship graph for them as a single Context whose
// subject is the namespace.
//
// Unlike the subject-centric resolvers, existence here is decided by membership:
// every listed resource is a node, and an edge target is "resolved" exactly when
// it is one of those nodes. This needs one List per kind (no per-reference Gets),
// so it scales to a whole namespace, and it still honestly flags a reference to
// something that is not there (for example an Ingress routing to a Service that
// does not exist). Cluster-scoped Nodes and PersistentVolumes are listed too so
// that runs-on and bound-to targets resolve.
func BuildNamespaceGraph(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
) (*Context, error) {
	var relations []Relation
	nodeSet := map[ResourceRef]struct{}{}

	ref := func(kind, name, ns string) ResourceRef {
		r := ResourceRef{Kind: kind, Name: name, Namespace: ns}
		nodeSet[r] = struct{}{}
		return r
	}

	opts := metav1.ListOptions{}

	deployments, err := clientset.AppsV1().Deployments(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	for i := range deployments.Items {
		d := &deployments.Items[i]
		relations = append(relations, ownerEdges(ref("Deployment", d.Name, namespace), d.OwnerReferences, namespace)...)
	}

	replicaSets, err := clientset.AppsV1().ReplicaSets(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list replicasets: %w", err)
	}
	for i := range replicaSets.Items {
		rs := &replicaSets.Items[i]
		relations = append(relations, ownerEdges(ref("ReplicaSet", rs.Name, namespace), rs.OwnerReferences, namespace)...)
	}

	statefulSets, err := clientset.AppsV1().StatefulSets(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}
	for i := range statefulSets.Items {
		sts := &statefulSets.Items[i]
		relations = append(relations, ownerEdges(ref("StatefulSet", sts.Name, namespace), sts.OwnerReferences, namespace)...)
	}

	daemonSets, err := clientset.AppsV1().DaemonSets(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list daemonsets: %w", err)
	}
	for i := range daemonSets.Items {
		ds := &daemonSets.Items[i]
		relations = append(relations, ownerEdges(ref("DaemonSet", ds.Name, namespace), ds.OwnerReferences, namespace)...)
	}

	jobs, err := clientset.BatchV1().Jobs(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	for i := range jobs.Items {
		j := &jobs.Items[i]
		relations = append(relations, ownerEdges(ref("Job", j.Name, namespace), j.OwnerReferences, namespace)...)
	}

	cronJobs, err := clientset.BatchV1().CronJobs(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list cronjobs: %w", err)
	}
	for i := range cronJobs.Items {
		cj := &cronJobs.Items[i]
		relations = append(relations, ownerEdges(ref("CronJob", cj.Name, namespace), cj.OwnerReferences, namespace)...)
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		podRef := ref("Pod", p.Name, namespace)
		relations = append(relations, ownerEdges(podRef, p.OwnerReferences, namespace)...)
		if p.Spec.NodeName != "" {
			relations = append(relations, Relation{
				From: podRef,
				Type: RunsOn,
				To:   ResourceRef{Kind: "Node", Name: p.Spec.NodeName},
			})
		}
		relations = append(relations, podConfigurationRelations(podRef, p)...)
		relations = append(relations, podStorageRelations(podRef, p)...)
	}

	services, err := clientset.CoreV1().Services(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	for i := range services.Items {
		s := &services.Items[i]
		svcRef := ref("Service", s.Name, namespace)
		relations = append(relations, selectsRelations(svcRef, s.Spec.Selector, pods.Items)...)
	}

	ingresses, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list ingresses: %w", err)
	}
	for i := range ingresses.Items {
		ing := &ingresses.Items[i]
		ref("Ingress", ing.Name, namespace)
		edges, err := ResolveIngress(ctx, clientset, ing)
		if err != nil {
			return nil, err
		}
		relations = append(relations, edges...)
	}

	configMaps, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list configmaps: %w", err)
	}
	for i := range configMaps.Items {
		ref("ConfigMap", configMaps.Items[i].Name, namespace)
	}

	secrets, err := clientset.CoreV1().Secrets(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	for i := range secrets.Items {
		ref("Secret", secrets.Items[i].Name, namespace)
	}

	pvcs, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumeclaims: %w", err)
	}
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		pvcRef := ref("PersistentVolumeClaim", pvc.Name, namespace)
		if pvc.Spec.VolumeName != "" {
			relations = append(relations, Relation{
				From: pvcRef,
				Type: BoundTo,
				To:   ResourceRef{Kind: "PersistentVolume", Name: pvc.Spec.VolumeName},
			})
		}
	}

	// Cluster-scoped resources, listed so runs-on and bound-to targets resolve.
	nodes, err := clientset.CoreV1().Nodes().List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	for i := range nodes.Items {
		ref("Node", nodes.Items[i].Name, "")
	}

	pvs, err := clientset.CoreV1().PersistentVolumes().List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list persistentvolumes: %w", err)
	}
	for i := range pvs.Items {
		ref("PersistentVolume", pvs.Items[i].Name, "")
	}

	existence := make(map[ResourceRef]bool, len(nodeSet))
	for r := range nodeSet {
		existence[r] = true
	}
	for _, rel := range relations {
		if _, ok := nodeSet[rel.To]; !ok {
			existence[rel.To] = false
		}
	}

	subject := ResourceRef{Kind: "Namespace", Name: namespace}
	return New(subject, relations, existence), nil
}
