package graph

import (
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// This file enriches graph nodes with compact, pre-formatted display attributes
// for the kinds Mantis already lists. Every string here is one line in the
// detail panel; the goal is to answer "what would kubectl describe show me that
// matters?" without dumping the whole object. Absent-but-significant facts (a
// Pod with no resource requests, no probes) are shown as "none" rather than
// omitted, because the gap is itself the signal an operator is looking for.

// podAttributes builds the Pod detail lines. The health note stays first because
// engine.statusFor derives the status ring from attrs[0]; everything after it is
// additive and safe to reorder.
func podAttributes(pod *corev1.Pod) []string {
	attrs := []string{podHealthNote(pod)}

	if img := containerImages(pod.Spec.Containers); img != "" {
		attrs = append(attrs, "image: "+img)
	}
	if pod.Status.QOSClass != "" {
		attrs = append(attrs, "qos: "+string(pod.Status.QOSClass))
	}

	req, lim := containerResources(pod.Spec.Containers)
	attrs = append(attrs, "requests: "+orNone(req), "limits: "+orNone(lim))
	attrs = append(attrs, "probes: "+orNone(probeSummary(pod.Spec.Containers)))

	if sa := pod.Spec.ServiceAccountName; sa != "" && sa != "default" {
		attrs = append(attrs, "serviceAccount: "+sa)
	}
	return attrs
}

// deploymentAttributes summarizes a Deployment: readiness, image, strategy, and
// the pod template's requests/limits/probes.
func deploymentAttributes(d *appsv1.Deployment) []string {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	attrs := []string{fmt.Sprintf("replicas: %d/%d ready", d.Status.ReadyReplicas, desired)}
	if d.Spec.Strategy.Type != "" {
		attrs = append(attrs, "strategy: "+string(d.Spec.Strategy.Type))
	}
	return append(attrs, templateAttributes(d.Spec.Template.Spec.Containers)...)
}

// replicaSetAttributes summarizes a ReplicaSet's readiness and pod template.
func replicaSetAttributes(rs *appsv1.ReplicaSet) []string {
	desired := int32(1)
	if rs.Spec.Replicas != nil {
		desired = *rs.Spec.Replicas
	}
	attrs := []string{fmt.Sprintf("replicas: %d/%d ready", rs.Status.ReadyReplicas, desired)}
	return append(attrs, templateAttributes(rs.Spec.Template.Spec.Containers)...)
}

// statefulSetAttributes summarizes a StatefulSet's readiness and pod template.
func statefulSetAttributes(sts *appsv1.StatefulSet) []string {
	desired := int32(1)
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}
	attrs := []string{fmt.Sprintf("replicas: %d/%d ready", sts.Status.ReadyReplicas, desired)}
	return append(attrs, templateAttributes(sts.Spec.Template.Spec.Containers)...)
}

// daemonSetAttributes summarizes a DaemonSet: desired vs ready per-node scheduling
// and the pod template.
func daemonSetAttributes(ds *appsv1.DaemonSet) []string {
	attrs := []string{fmt.Sprintf("scheduled: %d/%d ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)}
	return append(attrs, templateAttributes(ds.Spec.Template.Spec.Containers)...)
}

// jobAttributes summarizes a Job's completion count and pod template.
func jobAttributes(j *batchv1.Job) []string {
	desired := int32(1)
	if j.Spec.Completions != nil {
		desired = *j.Spec.Completions
	}
	attrs := []string{fmt.Sprintf("completions: %d/%d", j.Status.Succeeded, desired)}
	return append(attrs, templateAttributes(j.Spec.Template.Spec.Containers)...)
}

// cronJobAttributes summarizes a CronJob's schedule, suspension, and the pod
// template of the Job it creates.
func cronJobAttributes(cj *batchv1.CronJob) []string {
	attrs := []string{"schedule: " + cj.Spec.Schedule}
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		attrs = append(attrs, "suspended")
	}
	return append(attrs, templateAttributes(cj.Spec.JobTemplate.Spec.Template.Spec.Containers)...)
}

// templateAttributes renders the image / requests / limits / probes lines shared
// by every workload kind from its pod template's containers.
func templateAttributes(containers []corev1.Container) []string {
	var attrs []string
	if img := containerImages(containers); img != "" {
		attrs = append(attrs, "image: "+img)
	}
	req, lim := containerResources(containers)
	attrs = append(attrs, "requests: "+orNone(req), "limits: "+orNone(lim))
	attrs = append(attrs, "probes: "+orNone(probeSummary(containers)))
	return attrs
}

// serviceAttributes summarizes a Service (type, cluster IP, ports) and, crucially,
// the real endpoint addresses backing it — the data `kubectl get endpoints` shows
// — pulled from the same EndpointSlices used to build serves edges. Ready and
// not-ready backends are distinguished so a Service with zero ready endpoints is
// visibly broken.
func serviceAttributes(svc *corev1.Service, slices []discoveryv1.EndpointSlice) []string {
	attrs := []string{"type: " + string(svc.Spec.Type)}
	if ip := svc.Spec.ClusterIP; ip != "" && ip != corev1.ClusterIPNone {
		attrs = append(attrs, "clusterIP: "+ip)
	} else if ip == corev1.ClusterIPNone {
		attrs = append(attrs, "clusterIP: None (headless)")
	}
	if len(svc.Spec.ExternalIPs) > 0 {
		attrs = append(attrs, "externalIP: "+strings.Join(svc.Spec.ExternalIPs, ", "))
	}
	if ports := servicePorts(svc.Spec.Ports); ports != "" {
		attrs = append(attrs, "ports: "+ports)
	}
	attrs = append(attrs, "endpoints: "+endpointSummary(svc.Name, slices))
	return attrs
}

// servicePorts renders a Service's ports as "80→8080/TCP, 443→8443/TCP".
func servicePorts(ports []corev1.ServicePort) string {
	var parts []string
	for _, p := range ports {
		target := p.TargetPort.String()
		if target == "" || target == "0" {
			target = fmt.Sprintf("%d", p.Port)
		}
		proto := string(p.Protocol)
		if proto == "" {
			proto = "TCP"
		}
		seg := fmt.Sprintf("%d→%s/%s", p.Port, target, proto)
		if p.NodePort != 0 {
			seg += fmt.Sprintf(" (node %d)", p.NodePort)
		}
		parts = append(parts, seg)
	}
	return strings.Join(parts, ", ")
}

// endpointSummary lists the actual ready backend addresses (IP:port) for a
// Service from its EndpointSlices, e.g. "10.244.1.5:8080, 10.244.2.3:8080", and
// notes any not-ready addresses. "none" means the Service has no backends at all.
func endpointSummary(svcName string, slices []discoveryv1.EndpointSlice) string {
	var ready []string
	notReady := 0
	seen := map[string]struct{}{}

	for i := range slices {
		slice := &slices[i]
		if slice.Labels[serviceNameLabel] != svcName {
			continue
		}
		ports := slicePorts(slice.Ports)
		for _, ep := range slice.Endpoints {
			isReady := ep.Conditions.Ready == nil || *ep.Conditions.Ready
			for _, addr := range ep.Addresses {
				for _, port := range ports {
					key := fmt.Sprintf("%s:%s", addr, port)
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					if isReady {
						ready = append(ready, key)
					} else {
						notReady++
					}
				}
			}
		}
	}

	sort.Strings(ready)
	if len(ready) == 0 && notReady == 0 {
		return "none"
	}
	out := strings.Join(ready, ", ")
	if notReady > 0 {
		if out != "" {
			out += " "
		}
		out += fmt.Sprintf("(+%d not ready)", notReady)
	}
	return out
}

// slicePorts renders the numeric ports declared on an EndpointSlice, defaulting
// to a single empty string when the slice declares none (so an address still
// yields one entry).
func slicePorts(ports []discoveryv1.EndpointPort) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.Port != nil {
			out = append(out, fmt.Sprintf("%d", *p.Port))
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// nodeAttributes summarizes a cluster Node: readiness, role, kubelet version,
// OS/arch, allocatable CPU/memory, pod capacity, and any taints — the fields that
// decide whether and what can schedule there.
func nodeAttributes(node *corev1.Node) []string {
	var attrs []string

	ready := "NotReady"
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			ready = "Ready"
		}
	}
	attrs = append(attrs, "status: "+ready)

	if role := nodeRoles(node); role != "" {
		attrs = append(attrs, "roles: "+role)
	}
	if v := node.Status.NodeInfo.KubeletVersion; v != "" {
		attrs = append(attrs, "version: "+v)
	}
	if os := node.Status.NodeInfo.OperatingSystem; os != "" {
		attrs = append(attrs, "os: "+os+"/"+node.Status.NodeInfo.Architecture)
	}
	if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
		mem := node.Status.Allocatable[corev1.ResourceMemory]
		attrs = append(attrs, "allocatable: cpu "+cpu.String()+" · mem "+mem.String())
	}
	if pods, ok := node.Status.Capacity[corev1.ResourcePods]; ok {
		attrs = append(attrs, "pod capacity: "+pods.String())
	}
	if len(node.Spec.Taints) > 0 {
		var t []string
		for _, taint := range node.Spec.Taints {
			t = append(t, taint.Key+":"+string(taint.Effect))
		}
		attrs = append(attrs, "taints: "+strings.Join(t, ", "))
	}
	return attrs
}

// nodeRoles reads the well-known node-role.kubernetes.io/<role> labels.
func nodeRoles(node *corev1.Node) string {
	var roles []string
	const prefix = "node-role.kubernetes.io/"
	for k := range node.Labels {
		if strings.HasPrefix(k, prefix) {
			if r := strings.TrimPrefix(k, prefix); r != "" {
				roles = append(roles, r)
			}
		}
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

// resourceQuotaAttributes renders a ResourceQuota as "used / hard" per tracked
// resource — the namespace-level requests/limits governance an operator asks
// about.
func resourceQuotaAttributes(rq *corev1.ResourceQuota) []string {
	names := make([]string, 0, len(rq.Status.Hard))
	for name := range rq.Status.Hard {
		names = append(names, string(name))
	}
	sort.Strings(names)

	attrs := make([]string, 0, len(names))
	for _, name := range names {
		hard := rq.Status.Hard[corev1.ResourceName(name)]
		used := rq.Status.Used[corev1.ResourceName(name)]
		attrs = append(attrs, fmt.Sprintf("%s: %s / %s", name, used.String(), hard.String()))
	}
	if len(attrs) == 0 {
		return []string{"no tracked resources"}
	}
	return attrs
}

// limitRangeAttributes renders a LimitRange's per-type default/min/max settings —
// the namespace policy that fills in container requests/limits when a pod omits
// them.
func limitRangeAttributes(lr *corev1.LimitRange) []string {
	var attrs []string
	for _, item := range lr.Spec.Limits {
		if len(item.Default) > 0 {
			attrs = append(attrs, fmt.Sprintf("%s default limit: %s", item.Type, quantityMap(item.Default)))
		}
		if len(item.DefaultRequest) > 0 {
			attrs = append(attrs, fmt.Sprintf("%s default request: %s", item.Type, quantityMap(item.DefaultRequest)))
		}
		if len(item.Max) > 0 {
			attrs = append(attrs, fmt.Sprintf("%s max: %s", item.Type, quantityMap(item.Max)))
		}
		if len(item.Min) > 0 {
			attrs = append(attrs, fmt.Sprintf("%s min: %s", item.Type, quantityMap(item.Min)))
		}
	}
	if len(attrs) == 0 {
		return []string{"no limits defined"}
	}
	return attrs
}

// hpaAttributes summarizes a HorizontalPodAutoscaler: replica bounds, current
// scale, and the metrics it tracks.
func hpaAttributes(hpa *autoscalingv2.HorizontalPodAutoscaler) []string {
	min := int32(1)
	if hpa.Spec.MinReplicas != nil {
		min = *hpa.Spec.MinReplicas
	}
	attrs := []string{
		fmt.Sprintf("replicas: %d–%d", min, hpa.Spec.MaxReplicas),
		fmt.Sprintf("current: %d desired: %d", hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas),
	}
	if m := hpaMetrics(hpa.Spec.Metrics); m != "" {
		attrs = append(attrs, "metrics: "+m)
	}
	target := hpa.Spec.ScaleTargetRef
	attrs = append(attrs, fmt.Sprintf("target: %s/%s", target.Kind, target.Name))
	return attrs
}

// hpaMetrics renders the metric names an HPA scales on (e.g. "cpu, memory").
func hpaMetrics(metrics []autoscalingv2.MetricSpec) string {
	var out []string
	for _, m := range metrics {
		switch m.Type {
		case autoscalingv2.ResourceMetricSourceType:
			if m.Resource != nil {
				out = append(out, string(m.Resource.Name))
			}
		case autoscalingv2.PodsMetricSourceType:
			if m.Pods != nil {
				out = append(out, m.Pods.Metric.Name)
			}
		case autoscalingv2.ObjectMetricSourceType:
			if m.Object != nil {
				out = append(out, m.Object.Metric.Name)
			}
		case autoscalingv2.ExternalMetricSourceType:
			if m.External != nil {
				out = append(out, m.External.Metric.Name)
			}
		}
	}
	return strings.Join(out, ", ")
}

// containerImages joins the unique images across a container set, e.g.
// "nginx:1.27, sidecar:1.2".
func containerImages(containers []corev1.Container) string {
	var images []string
	seen := map[string]struct{}{}
	for _, c := range containers {
		if c.Image == "" {
			continue
		}
		if _, ok := seen[c.Image]; ok {
			continue
		}
		seen[c.Image] = struct{}{}
		images = append(images, c.Image)
	}
	return strings.Join(images, ", ")
}

// containerResources sums CPU and memory requests and limits across a container
// set and renders each as "cpu X · mem Y" (empty when nothing is set, so the
// caller can surface "none").
func containerResources(containers []corev1.Container) (requests, limits string) {
	var reqCPU, reqMem, limCPU, limMem resource.Quantity
	for _, c := range containers {
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			reqCPU.Add(q)
		}
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			reqMem.Add(q)
		}
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			limCPU.Add(q)
		}
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			limMem.Add(q)
		}
	}
	return quantityPair(reqCPU, reqMem), quantityPair(limCPU, limMem)
}

// quantityPair renders a cpu/memory pair, omitting whichever is zero.
func quantityPair(cpu, mem resource.Quantity) string {
	var parts []string
	if !cpu.IsZero() {
		parts = append(parts, "cpu "+cpu.String())
	}
	if !mem.IsZero() {
		parts = append(parts, "mem "+mem.String())
	}
	return strings.Join(parts, " · ")
}

// quantityMap renders a resource list (as found on LimitRange items) as
// "cpu=X, memory=Y", sorted for stable output.
func quantityMap(list corev1.ResourceList) string {
	names := make([]string, 0, len(list))
	for name := range list {
		names = append(names, string(name))
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		q := list[corev1.ResourceName(name)]
		parts = append(parts, name+"="+q.String())
	}
	return strings.Join(parts, ", ")
}

// probeSummary lists which probe types are configured across a container set,
// e.g. "liveness+readiness". Empty means no probes at all.
func probeSummary(containers []corev1.Container) string {
	var live, ready, startup bool
	for _, c := range containers {
		if c.LivenessProbe != nil {
			live = true
		}
		if c.ReadinessProbe != nil {
			ready = true
		}
		if c.StartupProbe != nil {
			startup = true
		}
	}
	var parts []string
	if live {
		parts = append(parts, "liveness")
	}
	if ready {
		parts = append(parts, "readiness")
	}
	if startup {
		parts = append(parts, "startup")
	}
	return strings.Join(parts, "+")
}

// orNone returns s, or the literal "none" when s is empty, so a missing-but-
// meaningful value (no requests, no probes) is shown rather than hidden.
func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
