// Package synth generates realistic synthetic Kubernetes clusters for
// benchmarks, tests and local UI development. It produces real Kubernetes API
// objects (Deployments with ReplicaSets and owned Pods, Services with matching
// EndpointSlices, ConfigMaps, Secrets, PVCs bound to PVs, RBAC, NetworkPolicies,
// HPAs, Ingresses...) rather than a pre-built graph, so whatever consumes them —
// typically a fake clientset fed to graph.BuildClusterGraph — exercises the real
// relationship resolvers end to end.
//
// Output is fully deterministic for a given Options value.
package synth

import (
	"fmt"
	"math/rand"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Options sizes the generated cluster.
type Options struct {
	// Resources is the approximate number of Kubernetes objects to generate.
	Resources int
	// Seed makes the output reproducible; the same seed gives the same cluster.
	Seed int64
}

// Generate returns the objects of a synthetic cluster of roughly
// opts.Resources resources: many namespaces, multiple worker nodes, several
// workload types, configuration and storage references, shared storage,
// cluster-scoped resources, cross-namespace RBAC grants, and a few
// deliberately broken references (a missing ConfigMap, a selector that
// matches nothing) so dangling-reference handling is exercised too.
func Generate(opts Options) []runtime.Object {
	if opts.Resources < 30 {
		opts.Resources = 30
	}
	g := &gen{rng: rand.New(rand.NewSource(opts.Seed)), budget: opts.Resources}
	return g.run()
}

type gen struct {
	rng    *rand.Rand
	budget int
	objs   []runtime.Object
	uid    int
	nodes  []string
	pvSeq  int
	events int // Events generated (not counted against budget)
}

func (g *gen) add(o runtime.Object) { g.objs = append(g.objs, o) }

// event records a Kubernetes Event about an object, timestamped after the
// objects' creation times so it reads as recent cluster history.
func (g *gen) event(ns, kind, name, typ, reason, msg string, count int32) {
	m := g.meta(fmt.Sprintf("%s.%x", name, g.uid), ns, nil)
	at := metav1.Unix(1767225600+30*86400+int64(g.uid)*7, 0)
	g.events++ // events are history, not resources: they don't use the budget
	g.add(&corev1.Event{
		ObjectMeta:     m,
		InvolvedObject: corev1.ObjectReference{Kind: kind, Name: name, Namespace: ns},
		Type:           typ,
		Reason:         reason,
		Message:        msg,
		Count:          count,
		Source:         corev1.EventSource{Component: "kubelet"},
		FirstTimestamp: at,
		LastTimestamp:  at,
	})
}

func (g *gen) meta(name, ns string, labels map[string]string) metav1.ObjectMeta {
	g.uid++
	return metav1.ObjectMeta{
		Name:              name,
		Namespace:         ns,
		UID:               types.UID(fmt.Sprintf("00000000-0000-4000-8000-%012d", g.uid)),
		ResourceVersion:   "1",
		Labels:            labels,
		CreationTimestamp: metav1.Unix(1767225600+int64(g.uid), 0), // 2026-01-01 onward
	}
}

var (
	teams   = []string{"shop", "catalog", "checkout", "payments", "search", "identity", "billing", "analytics", "media", "notify", "ledger", "inventory", "shipping", "reviews", "pricing", "ads"}
	tiers   = []string{"api", "web", "worker", "db", "cache", "gateway", "sync", "indexer"}
	storage = []string{"standard", "fast-ssd", "shared-nfs"}
)

func (g *gen) run() []runtime.Object {
	nNodes := clamp(g.budget/150, 3, 60)
	nNamespaces := clamp(g.budget/180, 3, 80)

	// Cluster-scoped infrastructure.
	for i := 0; i < nNodes; i++ {
		name := fmt.Sprintf("worker-%02d", i+1)
		g.nodes = append(g.nodes, name)
		ready := corev1.ConditionTrue
		if i == nNodes-1 && nNodes > 4 {
			ready = corev1.ConditionFalse // one unhealthy node, as in real life
		}
		g.add(&corev1.Node{
			ObjectMeta: g.meta(name, "", map[string]string{"kubernetes.io/hostname": name, "topology.kubernetes.io/zone": fmt.Sprintf("zone-%c", 'a'+i%3)}),
			Status: corev1.NodeStatus{
				Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
				Capacity:    corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8"), corev1.ResourceMemory: resource.MustParse("32Gi"), corev1.ResourcePods: resource.MustParse("110")},
				Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("7910m"), corev1.ResourceMemory: resource.MustParse("30Gi"), corev1.ResourcePods: resource.MustParse("110")},
				NodeInfo:    corev1.NodeSystemInfo{KubeletVersion: "v1.31.4"},
			},
		})
	}
	reclaim := corev1.PersistentVolumeReclaimDelete
	for _, sc := range storage {
		g.add(&storagev1.StorageClass{ObjectMeta: g.meta(sc, "", nil), Provisioner: "csi.synthetic.local", ReclaimPolicy: &reclaim})
	}
	g.add(&rbacv1.ClusterRole{ObjectMeta: g.meta("metrics-reader", "", nil), Rules: []rbacv1.PolicyRule{{Verbs: []string{"get", "list", "watch"}, APIGroups: []string{""}, Resources: []string{"pods", "nodes"}}}})
	g.add(&rbacv1.ClusterRole{ObjectMeta: g.meta("system:aggregate-to-view", "", nil)}) // system noise, hidden

	// A monitoring namespace whose ServiceAccount is granted read access
	// cluster-wide and inside every app namespace (cross-namespace RBAC).
	g.namespace("monitoring")
	g.workload("monitoring", "prometheus", "StatefulSet", 1, true)
	g.add(&rbacv1.ClusterRoleBinding{
		ObjectMeta: g.meta("prometheus-metrics", "", nil),
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "metrics-reader"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "prometheus", Namespace: "monitoring"}},
	})
	g.namespace("kube-system")
	g.workload("kube-system", "node-agent", "DaemonSet", 0, false)
	g.workload("kube-system", "coredns", "Deployment", 2, false)

	nsNames := make([]string, 0, nNamespaces)
	for i := 0; i < nNamespaces; i++ {
		team := teams[i%len(teams)]
		name := team
		if i >= len(teams) {
			name = fmt.Sprintf("%s-%d", team, i/len(teams)+1)
		}
		nsNames = append(nsNames, name)
		g.namespace(name)
	}

	// Fill namespaces round-robin with apps until the budget is spent.
	app := 0
	for len(g.objs)-g.events < g.budget {
		ns := nsNames[app%len(nsNames)]
		tier := tiers[(app/len(nsNames))%len(tiers)]
		name := fmt.Sprintf("%s-%s", ns, tier)
		if app >= len(nsNames)*len(tiers) {
			name = fmt.Sprintf("%s-%s-%d", ns, tier, app/(len(nsNames)*len(tiers)))
		}
		kind := "Deployment"
		switch {
		case tier == "db":
			kind = "StatefulSet"
		case tier == "sync" && app%3 == 0:
			kind = "CronJob"
		}
		replicas := 1 + g.rng.Intn(4)
		g.workload(ns, name, kind, replicas, tier == "api" || tier == "web" || tier == "gateway")
		if app%len(nsNames) == 0 && app/len(nsNames) == 1 {
			// Cross-namespace grant: monitoring may read this namespace.
			g.add(&rbacv1.RoleBinding{
				ObjectMeta: g.meta("monitoring-view", ns, nil),
				RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "metrics-reader"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "prometheus", Namespace: "monitoring"}},
			})
		}
		app++
	}
	return g.objs
}

func (g *gen) namespace(name string) {
	g.add(&corev1.Namespace{ObjectMeta: g.meta(name, "", map[string]string{"kubernetes.io/metadata.name": name})})
	g.add(&corev1.ServiceAccount{ObjectMeta: g.meta("default", name, nil)})
	g.add(&networkingv1.NetworkPolicy{
		ObjectMeta: g.meta("default-deny-ingress", name, nil),
		Spec:       networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}},
	})
}

// workload emits one application: its controller chain and Pods, a Service and
// EndpointSlice, configuration, identity and (for databases) storage.
func (g *gen) workload(ns, name, kind string, replicas int, public bool) {
	labels := map[string]string{"app.kubernetes.io/name": name, "app.kubernetes.io/part-of": ns}
	cm := name + "-config"
	secret := name + "-credentials"
	sa := name

	g.add(&corev1.ConfigMap{ObjectMeta: g.meta(cm, ns, nil), Data: map[string]string{"LOG_LEVEL": "info"}})
	g.add(&corev1.Secret{ObjectMeta: g.meta(secret, ns, nil), Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"password": []byte("synthetic-not-a-secret")}})
	g.add(&corev1.ServiceAccount{ObjectMeta: g.meta(sa, ns, nil)})
	if g.rng.Intn(3) == 0 {
		g.add(&rbacv1.Role{ObjectMeta: g.meta(name+"-reader", ns, nil), Rules: []rbacv1.PolicyRule{{Verbs: []string{"get", "list"}, APIGroups: []string{""}, Resources: []string{"configmaps"}}}})
		g.add(&rbacv1.RoleBinding{
			ObjectMeta: g.meta(name+"-reader", ns, nil),
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: name + "-reader"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa, Namespace: ns}},
		})
	}

	spec := corev1.PodSpec{
		ServiceAccountName: sa,
		Containers: []corev1.Container{{
			Name:  "app",
			Image: "registry.synthetic.local/" + name + ":1.0.0",
			EnvFrom: []corev1.EnvFromSource{
				{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: cm}}},
			},
			Env: []corev1.EnvVar{{Name: "DB_PASSWORD", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secret}, Key: "password"}}}},
		}},
	}
	if g.rng.Intn(40) == 0 {
		// A broken reference: the ConfigMap this mounts does not exist.
		spec.Volumes = append(spec.Volumes, corev1.Volume{Name: "feature-flags", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name + "-feature-flags"}}}})
	}

	var podNames []string
	ownerKind, ownerName := kind, name
	switch kind {
	case "Deployment":
		r := int32(replicas)
		g.add(&appsv1.Deployment{ObjectMeta: g.meta(name, ns, labels), Spec: appsv1.DeploymentSpec{Replicas: &r, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{Spec: spec}},
			Status: appsv1.DeploymentStatus{Replicas: r, ReadyReplicas: r, AvailableReplicas: r}})
		rs := fmt.Sprintf("%s-%07x", name, g.rng.Intn(1<<28))
		rsMeta := g.meta(rs, ns, labels)
		rsMeta.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: name, Controller: ptr(true)}}
		g.add(&appsv1.ReplicaSet{ObjectMeta: rsMeta, Spec: appsv1.ReplicaSetSpec{Replicas: &r}, Status: appsv1.ReplicaSetStatus{Replicas: r, ReadyReplicas: r}})
		ownerKind, ownerName = "ReplicaSet", rs
		if g.rng.Intn(5) == 0 {
			g.add(&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: g.meta(name, ns, nil), Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: name}, MinReplicas: ptr(int32(1)), MaxReplicas: 10}})
		}
	case "StatefulSet":
		r := int32(max(replicas, 1))
		replicas = int(r)
		g.add(&appsv1.StatefulSet{ObjectMeta: g.meta(name, ns, labels), Spec: appsv1.StatefulSetSpec{Replicas: &r, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{Spec: spec}},
			Status: appsv1.StatefulSetStatus{Replicas: r, ReadyReplicas: r}})
	case "DaemonSet":
		g.add(&appsv1.DaemonSet{ObjectMeta: g.meta(name, ns, labels), Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{Spec: spec}},
			Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: int32(len(g.nodes)), NumberReady: int32(len(g.nodes))}})
		replicas = len(g.nodes)
	case "CronJob":
		g.add(&batchv1.CronJob{ObjectMeta: g.meta(name, ns, labels), Spec: batchv1.CronJobSpec{Schedule: "*/15 * * * *", JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: spec}}}}})
		job := fmt.Sprintf("%s-%d", name, 29000000+g.rng.Intn(99999))
		jm := g.meta(job, ns, labels)
		jm.OwnerReferences = []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "CronJob", Name: name, Controller: ptr(true)}}
		g.add(&batchv1.Job{ObjectMeta: jm, Status: batchv1.JobStatus{Succeeded: 1}})
		ownerKind, ownerName = "Job", job
		replicas = 1
	}

	for i := 0; i < replicas; i++ {
		var pod string
		switch kind {
		case "StatefulSet":
			pod = fmt.Sprintf("%s-%d", name, i)
		case "DaemonSet":
			pod = fmt.Sprintf("%s-%05x", name, g.rng.Intn(1<<20))
		default:
			pod = fmt.Sprintf("%s-%05x", ownerName, g.rng.Intn(1<<20))
		}
		podNames = append(podNames, pod)
		ps := *spec.DeepCopy()
		if kind == "StatefulSet" {
			claim := "data-" + pod
			ps.Volumes = append(ps.Volumes, corev1.Volume{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim}}})
			g.claim(ns, claim, storage[g.pvSeq%2], corev1.ReadWriteOnce)
		}
		node := g.nodes[g.rng.Intn(len(g.nodes))]
		if kind == "DaemonSet" {
			node = g.nodes[i]
		}
		ps.NodeName = node
		pm := g.meta(pod, ns, labels)
		pm.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: ownerKind, Name: ownerName, Controller: ptr(true)}}
		phase, ready := corev1.PodRunning, true
		if kind == "CronJob" {
			phase, ready = corev1.PodSucceeded, false
		} else if g.rng.Intn(30) == 0 {
			ready = false // the occasional crash-looping replica
		}
		cs := corev1.ContainerStatus{Name: "app", Ready: ready, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
		if !ready && phase == corev1.PodRunning {
			cs.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
			cs.RestartCount = int32(3 + g.rng.Intn(20))
		}
		if phase == corev1.PodSucceeded {
			cs.State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}}
		}
		g.add(&corev1.Pod{ObjectMeta: pm, Spec: ps, Status: corev1.PodStatus{Phase: phase, ContainerStatuses: []corev1.ContainerStatus{cs}}})
		// The events a kubelet would have recorded for this Pod.
		if cs.State.Waiting != nil {
			g.event(ns, "Pod", pod, corev1.EventTypeWarning, "BackOff", "Back-off restarting failed container app in pod "+pod, cs.RestartCount)
		} else if i == 0 && phase == corev1.PodRunning {
			g.event(ns, "Pod", pod, corev1.EventTypeNormal, "Pulled", "Container image \"registry.synthetic.local/"+name+":1.4.2\" already present on machine", 1)
			g.event(ns, "Pod", pod, corev1.EventTypeNormal, "Started", "Started container app", 1)
		}
	}

	if kind == "CronJob" || kind == "DaemonSet" {
		return
	}
	svc := name
	g.add(&corev1.Service{ObjectMeta: g.meta(svc, ns, nil), Spec: corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}}}})
	eps := make([]discoveryv1.Endpoint, 0, len(podNames))
	for _, p := range podNames {
		eps = append(eps, discoveryv1.Endpoint{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: ptr(true)},
			TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: p, Namespace: ns}})
	}
	g.add(&discoveryv1.EndpointSlice{ObjectMeta: g.meta(svc+"-abcde", ns, map[string]string{discoveryv1.LabelServiceName: svc}), AddressType: discoveryv1.AddressTypeIPv4, Endpoints: eps})
	if public {
		pt := networkingv1.PathTypePrefix
		g.add(&networkingv1.Ingress{ObjectMeta: g.meta(name, ns, nil), Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			Host: name + ".synthetic.local",
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
				Path: "/", PathType: &pt, Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: svc, Port: networkingv1.ServiceBackendPort{Number: 80}}},
			}}}},
		}}}})
		g.add(&networkingv1.NetworkPolicy{
			ObjectMeta: g.meta(name+"-allow-http", ns, nil),
			Spec:       networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: labels}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}},
		})
	}
}

// claim emits a bound PVC and its PV.
func (g *gen) claim(ns, name, class string, mode corev1.PersistentVolumeAccessMode) {
	g.pvSeq++
	pv := fmt.Sprintf("pvc-%08d", g.pvSeq)
	g.add(&corev1.PersistentVolumeClaim{ObjectMeta: g.meta(name, ns, nil), Spec: corev1.PersistentVolumeClaimSpec{
		StorageClassName: &class, VolumeName: pv, AccessModes: []corev1.PersistentVolumeAccessMode{mode},
		Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
	}, Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}})
	g.add(&corev1.PersistentVolume{ObjectMeta: g.meta(pv, "", nil), Spec: corev1.PersistentVolumeSpec{
		StorageClassName: class, AccessModes: []corev1.PersistentVolumeAccessMode{mode},
		Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
		ClaimRef: &corev1.ObjectReference{Kind: "PersistentVolumeClaim", Name: name, Namespace: ns},
	}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}})
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func ptr[T any](v T) *T { return &v }

// NodeMetrics returns metrics.k8s.io NodeMetrics for the synthetic cluster's
// Nodes, as metrics-server would report them: deterministic per seed, using
// 20–70% of each node's allocatable CPU and memory.
func NodeMetrics(objs []runtime.Object, seed int64) []runtime.Object {
	rng := rand.New(rand.NewSource(seed + 1000))
	var out []runtime.Object
	for _, o := range objs {
		n, ok := o.(*corev1.Node)
		if !ok {
			continue
		}
		cpu := n.Status.Allocatable.Cpu().MilliValue() * int64(20+rng.Intn(50)) / 100
		mem := n.Status.Allocatable.Memory().Value() * int64(20+rng.Intn(50)) / 100
		out = append(out, &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "metrics.k8s.io/v1beta1",
			"kind":       "NodeMetrics",
			"metadata":   map[string]interface{}{"name": n.Name},
			"timestamp":  "2026-01-31T00:00:00Z",
			"window":     "20s",
			"usage": map[string]interface{}{
				"cpu":    fmt.Sprintf("%dm", cpu),
				"memory": fmt.Sprintf("%dKi", mem/1024),
			},
		}})
	}
	return out
}
