package report

import (
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// Level is a health or severity level. Every level shown in a report is
// derived from an observed status field — never from the mere existence of an
// object.
type Level int

const (
	Healthy Level = iota
	Warning
	Critical
	Unknown
	Informational
)

func (l Level) String() string {
	return [...]string{"Healthy", "Warning", "Critical", "Unknown", "Informational"}[l]
}

// PodState is a Pod's observed state.
type PodState struct {
	Phase, Reason     string
	Ready, Containers int
	Restarts          int32
	Node, IP, Owner   string
	Level             Level
}

// ObservePod derives a Pod's state from its status.
func ObservePod(p *corev1.Pod) PodState {
	st := PodState{Phase: string(p.Status.Phase), Containers: len(p.Spec.Containers), Node: p.Spec.NodeName, IP: p.Status.PodIP}
	if st.Phase == "" {
		st.Phase = "Unknown"
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			st.Ready++
		}
		st.Restarts += cs.RestartCount
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && st.Reason == "" {
			st.Reason = cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" && st.Reason == "" && p.Status.Phase != corev1.PodSucceeded {
			st.Reason = cs.State.Terminated.Reason
		}
	}
	if st.Reason == "" {
		st.Reason = p.Status.Reason
	}
	if len(p.OwnerReferences) > 0 {
		st.Owner = p.OwnerReferences[0].Kind + "/" + p.OwnerReferences[0].Name
	}
	switch p.Status.Phase {
	case corev1.PodSucceeded:
		st.Level = Healthy
	case corev1.PodRunning:
		switch {
		case st.Containers > 0 && st.Ready == st.Containers:
			st.Level = Healthy
		case isCrashReason(st.Reason):
			st.Level = Critical
		default:
			st.Level = Warning
		}
	case corev1.PodPending:
		if isCrashReason(st.Reason) {
			st.Level = Critical
		} else {
			st.Level = Warning
		}
	case corev1.PodFailed:
		st.Level = Critical
	default:
		st.Level = Unknown
	}
	return st
}

func isCrashReason(r string) bool {
	switch r {
	case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "InvalidImageName", "CreateContainerError", "RunContainerError", "OOMKilled":
		return true
	}
	return false
}

// WorkloadState is a controller's observed replica state.
type WorkloadState struct {
	Kind, Namespace, Name     string
	Desired, Ready, Available int32
	Level                     Level
	Note                      string
}

func replicaLevel(desired, ready, available int32) (Level, string) {
	switch {
	case desired == 0:
		return Unknown, "scaled to 0"
	case ready >= desired && available >= desired:
		return Healthy, ""
	case ready == 0:
		return Critical, "no ready replicas"
	default:
		return Warning, fmt.Sprintf("%d of %d replicas ready", ready, desired)
	}
}

func deploymentState(d *appsv1.Deployment) WorkloadState {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	w := WorkloadState{Kind: "Deployment", Namespace: d.Namespace, Name: d.Name, Desired: desired, Ready: d.Status.ReadyReplicas, Available: d.Status.AvailableReplicas}
	w.Level, w.Note = replicaLevel(w.Desired, w.Ready, w.Available)
	return w
}

func statefulSetState(s *appsv1.StatefulSet) WorkloadState {
	desired := int32(1)
	if s.Spec.Replicas != nil {
		desired = *s.Spec.Replicas
	}
	w := WorkloadState{Kind: "StatefulSet", Namespace: s.Namespace, Name: s.Name, Desired: desired, Ready: s.Status.ReadyReplicas, Available: s.Status.AvailableReplicas}
	if w.Available == 0 && w.Ready > 0 {
		w.Available = w.Ready // older API servers do not report availableReplicas
	}
	w.Level, w.Note = replicaLevel(w.Desired, w.Ready, w.Available)
	return w
}

func daemonSetState(d *appsv1.DaemonSet) WorkloadState {
	w := WorkloadState{Kind: "DaemonSet", Namespace: d.Namespace, Name: d.Name, Desired: d.Status.DesiredNumberScheduled, Ready: d.Status.NumberReady, Available: d.Status.NumberAvailable}
	w.Level, w.Note = replicaLevel(w.Desired, w.Ready, w.Available)
	return w
}

// NodeState is a Node's observed readiness.
type NodeState struct {
	Name, Ready, Roles, Version, Pressure string
	Level                                 Level
}

func observeNode(n *corev1.Node) NodeState {
	st := NodeState{Name: n.Name, Ready: "Unknown", Version: n.Status.NodeInfo.KubeletVersion, Level: Unknown}
	var roles, pressure []string
	for k := range n.Labels {
		if strings.HasPrefix(k, "node-role.kubernetes.io/") {
			roles = append(roles, strings.TrimPrefix(k, "node-role.kubernetes.io/"))
		}
	}
	sort.Strings(roles)
	st.Roles = strings.Join(roles, ", ")
	for _, c := range n.Status.Conditions {
		switch {
		case c.Type == corev1.NodeReady:
			st.Ready = string(c.Status)
			if c.Status == corev1.ConditionTrue {
				st.Ready, st.Level = "Ready", Healthy
			} else if c.Status == corev1.ConditionFalse {
				st.Ready, st.Level = "NotReady", Critical
			}
		case c.Status == corev1.ConditionTrue && strings.HasSuffix(string(c.Type), "Pressure"), c.Type == corev1.NodeNetworkUnavailable && c.Status == corev1.ConditionTrue:
			pressure = append(pressure, string(c.Type))
		}
	}
	st.Pressure = strings.Join(pressure, ", ")
	if st.Level == Healthy && len(pressure) > 0 {
		st.Level = Warning
	}
	return st
}

// Health is the consolidated health summary.
type Health struct {
	Pods                                 map[string]int // Running, NotReady, Pending, Failed, Succeeded, Unknown
	PodsWithRestarts                     int
	PodLevels                            map[Level]int
	Workloads                            []WorkloadState
	WorkloadLevels                       map[Level]int
	Nodes                                []NodeState
	NodeLevels                           map[Level]int
	DeploymentsReady, DeploymentsTotal   int
	StatefulSetsReady, StatefulSetsTotal int
	DaemonSetsReady, DaemonSetsTotal     int
}

// ComputeHealth derives the health summary from observed status.
func ComputeHealth(s *Snapshot) Health {
	h := Health{Pods: map[string]int{}, PodLevels: map[Level]int{}, WorkloadLevels: map[Level]int{}, NodeLevels: map[Level]int{}}
	for i := range s.Pods {
		st := ObservePod(&s.Pods[i])
		switch {
		case st.Phase == "Running" && st.Level == Healthy:
			h.Pods["Running"]++
		case st.Phase == "Running":
			h.Pods["Running, not ready"]++
		default:
			h.Pods[st.Phase]++
		}
		if st.Restarts > 0 {
			h.PodsWithRestarts++
		}
		h.PodLevels[st.Level]++
	}
	add := func(w WorkloadState, ready, total *int) {
		h.Workloads = append(h.Workloads, w)
		h.WorkloadLevels[w.Level]++
		*total++
		if w.Level == Healthy {
			*ready++
		}
	}
	for i := range s.Deployments {
		add(deploymentState(&s.Deployments[i]), &h.DeploymentsReady, &h.DeploymentsTotal)
	}
	for i := range s.StatefulSets {
		add(statefulSetState(&s.StatefulSets[i]), &h.StatefulSetsReady, &h.StatefulSetsTotal)
	}
	for i := range s.DaemonSets {
		add(daemonSetState(&s.DaemonSets[i]), &h.DaemonSetsReady, &h.DaemonSetsTotal)
	}
	for i := range s.Nodes {
		n := observeNode(&s.Nodes[i])
		h.Nodes = append(h.Nodes, n)
		h.NodeLevels[n.Level]++
	}
	return h
}

// Finding is one detected anomaly.
type Finding struct {
	Severity                     Level
	Title, Kind, Namespace, Name string
	Description, Related         string
}

// ComputeFindings lists anomalies Mantis actually detects: broken and
// unverifiable references, unavailable workloads, failing or pending Pods,
// Services that select nothing, unready Nodes, unbound claims, Pods with no
// controller, and kinds Mantis could not read. It makes no security,
// performance or compliance claims.
func ComputeFindings(s *Snapshot, h Health) []Finding {
	var out []Finding
	add := func(f Finding) { out = append(out, f) }

	podLevel := map[string]Level{}
	for i := range s.Pods {
		p := &s.Pods[i]
		podLevel[p.Namespace+"/Pod/"+p.Name] = ObservePod(p).Level
	}
	// Broken and unverifiable references, from the graph.
	for _, n := range s.Graph.Nodes {
		if n.Resolved {
			continue
		}
		for _, e := range s.In(n.ID) {
			src := s.Node(e.From)
			if src == nil {
				continue
			}
			if n.Unverified {
				add(Finding{Severity: Unknown, Title: "Reference could not be verified", Kind: src.Kind, Namespace: src.Namespace, Name: src.Name,
					Description: fmt.Sprintf("%s %s %s/%s, which Mantis could not check (its kind is not listed, or reading it is not permitted).", src.Kind, e.Type, n.Kind, n.Name),
					Related:     n.Kind + "/" + n.Name})
				continue
			}
			sev := Warning
			if src.Kind == "Ingress" || (src.Kind == "Pod" && podLevel[src.ID] != Healthy) {
				sev = Critical
			}
			add(Finding{Severity: sev, Title: "Dangling reference", Kind: src.Kind, Namespace: src.Namespace, Name: src.Name,
				Description: fmt.Sprintf("%s %s %s/%s, which does not exist in the cluster.", src.Kind, e.Type, n.Kind, n.Name),
				Related:     n.Kind + "/" + n.Name})
		}
	}
	for _, w := range h.Workloads {
		if w.Level == Critical || w.Level == Warning {
			add(Finding{Severity: w.Level, Title: "Workload has unavailable replicas", Kind: w.Kind, Namespace: w.Namespace, Name: w.Name,
				Description: fmt.Sprintf("Desired %d, ready %d, available %d.", w.Desired, w.Ready, w.Available)})
		}
	}
	for i := range s.Pods {
		p := &s.Pods[i]
		st := ObservePod(p)
		switch {
		case st.Phase == "Failed":
			add(Finding{Severity: Critical, Title: "Pod failed", Kind: "Pod", Namespace: p.Namespace, Name: p.Name, Description: "Phase Failed" + reasonSuffix(st.Reason) + ".", Related: st.Owner})
		case isCrashReason(st.Reason):
			add(Finding{Severity: Critical, Title: "Pod cannot run", Kind: "Pod", Namespace: p.Namespace, Name: p.Name, Description: fmt.Sprintf("Container waiting: %s (restarts: %d).", st.Reason, st.Restarts), Related: st.Owner})
		case st.Phase == "Pending":
			add(Finding{Severity: Warning, Title: "Pod pending", Kind: "Pod", Namespace: p.Namespace, Name: p.Name, Description: "Phase Pending" + reasonSuffix(st.Reason) + ".", Related: st.Owner})
		case st.Phase == "Running" && st.Level != Healthy:
			add(Finding{Severity: Warning, Title: "Pod not ready", Kind: "Pod", Namespace: p.Namespace, Name: p.Name, Description: fmt.Sprintf("%d of %d containers ready.", st.Ready, st.Containers), Related: st.Owner})
		case st.Restarts >= 5:
			add(Finding{Severity: Warning, Title: "Pod restarting", Kind: "Pod", Namespace: p.Namespace, Name: p.Name, Description: fmt.Sprintf("%d container restarts.", st.Restarts), Related: st.Owner})
		}
		if len(p.OwnerReferences) == 0 && p.Annotations["kubernetes.io/config.mirror"] == "" && s.InGraph("Pod", p.Namespace, p.Name) {
			add(Finding{Severity: Informational, Title: "Pod not managed by a controller", Kind: "Pod", Namespace: p.Namespace, Name: p.Name,
				Description: "No owner reference: if it is deleted or its node fails, nothing recreates it."})
		}
	}
	for i := range s.Services {
		sv := &s.Services[i]
		if len(sv.Spec.Selector) == 0 || !s.InGraph("Service", sv.Namespace, sv.Name) {
			continue
		}
		selects := 0
		for _, e := range s.Out(sv.Namespace + "/Service/" + sv.Name) {
			if e.Type == "selects" {
				selects++
			}
		}
		if selects == 0 {
			add(Finding{Severity: Warning, Title: "Service selects no Pods", Kind: "Service", Namespace: sv.Namespace, Name: sv.Name,
				Description: "Its label selector matches no Pod in the namespace, so it has no backends."})
		}
	}
	for _, n := range h.Nodes {
		switch n.Level {
		case Critical:
			add(Finding{Severity: Critical, Title: "Node not ready", Kind: "Node", Name: n.Name, Description: "Ready condition is False."})
		case Unknown:
			add(Finding{Severity: Unknown, Title: "Node readiness unknown", Kind: "Node", Name: n.Name, Description: "The kubelet has not reported Ready status."})
		case Warning:
			add(Finding{Severity: Warning, Title: "Node under pressure", Kind: "Node", Name: n.Name, Description: "Conditions: " + n.Pressure + "."})
		}
	}
	for i := range s.PVCs {
		c := &s.PVCs[i]
		switch c.Status.Phase {
		case corev1.ClaimPending:
			add(Finding{Severity: Warning, Title: "Volume claim not bound", Kind: "PersistentVolumeClaim", Namespace: c.Namespace, Name: c.Name, Description: "Phase Pending: no volume has been bound to this claim."})
		case corev1.ClaimLost:
			add(Finding{Severity: Critical, Title: "Volume claim lost", Kind: "PersistentVolumeClaim", Namespace: c.Namespace, Name: c.Name, Description: "Phase Lost: the bound volume no longer exists."})
		}
	}
	for _, r := range s.NotReadable {
		add(Finding{Severity: Unknown, Title: "Resource kind not readable", Kind: r, Description: "Mantis is not permitted to list " + r + "; this report cannot describe them."})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := sevRank(out[i].Severity), sevRank(out[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func sevRank(l Level) int {
	return map[Level]int{Critical: 0, Warning: 1, Unknown: 2, Informational: 3, Healthy: 4}[l]
}

func reasonSuffix(r string) string {
	if r == "" {
		return ""
	}
	return " (" + r + ")"
}
