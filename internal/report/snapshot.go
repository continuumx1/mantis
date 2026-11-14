// Package report builds Mantis Infrastructure Reports: a password-protected
// PDF describing a cluster — overview, health, topology, inventory and
// findings. It runs in its own service (cmd/mantis-report) and consumes
// mantis-engine's internal snapshot API; it has no Kubernetes client and
// never modifies anything.
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/continuumx1/mantis/internal/snapshotapi"
)

// Graph is the engine's canonical graph as the report consumes it (a copy of
// the published GraphDTO, so this package does not depend on the engine).
type Graph struct {
	Context, Server, ClusterID, Version, SyncMode string
	Revision                                      uint64
	Namespaces                                    []string
	Skipped                                       []string
	Nodes                                         []Node
	Edges                                         []Edge
}

// Node is one resource in the graph.
type Node struct {
	ID, Kind, Name, Namespace, Status string
	Resolved, Unverified              bool
	Attributes                        []string
}

// Edge is one typed relationship.
type Edge struct {
	From, To, Type, Category, Basis, Evidence string
}

// Key identifies an edge the way the UI does ("from|type|to").
func (e Edge) Key() string { return e.From + "|" + e.Type + "|" + e.To }

// Snapshot is everything one report is built from, read once, so every
// section describes the same moment.
type Snapshot struct {
	Graph   Graph
	TakenAt time.Time
	byID    map[string]*Node
	edges   map[string]*Edge
	out     map[string][]*Edge
	in      map[string][]*Edge

	Namespaces          []corev1.Namespace
	Pods                []corev1.Pod
	Deployments         []appsv1.Deployment
	ReplicaSets         []appsv1.ReplicaSet
	StatefulSets        []appsv1.StatefulSet
	DaemonSets          []appsv1.DaemonSet
	Jobs                []batchv1.Job
	CronJobs            []batchv1.CronJob
	Services            []corev1.Service
	Ingresses           []networkingv1.Ingress
	NetworkPolicies     []networkingv1.NetworkPolicy
	ConfigMaps          []corev1.ConfigMap
	Secrets             []corev1.Secret // metadata and type only; values are wiped on read
	PVCs                []corev1.PersistentVolumeClaim
	PVs                 []corev1.PersistentVolume
	StorageClasses      []storagev1.StorageClass
	Nodes               []corev1.Node
	ServiceAccounts     []corev1.ServiceAccount
	Roles               []rbacv1.Role
	RoleBindings        []rbacv1.RoleBinding
	ClusterRoles        []rbacv1.ClusterRole
	ClusterRoleBindings []rbacv1.ClusterRoleBinding
	HPAs                []autoscalingv2.HorizontalPodAutoscaler
	ResourceQuotas      []corev1.ResourceQuota
	LimitRanges         []corev1.LimitRange
	// NotReadable lists kinds the engine's identity may not list.
	NotReadable []string
}

// Node returns a graph node by ID.
func (s *Snapshot) Node(id string) *Node { return s.byID[id] }

// Edge returns a graph edge by key.
func (s *Snapshot) Edge(key string) *Edge { return s.edges[key] }

// InGraph reports whether a resource is part of the visible graph — the same
// set the app shows (system-managed noise is excluded there, and here).
func (s *Snapshot) InGraph(kind, ns, name string) bool {
	_, ok := s.byID[ns+"/"+kind+"/"+name]
	return ok
}

// Out / In return a resource's outgoing and incoming edges.
func (s *Snapshot) Out(id string) []*Edge { return s.out[id] }
func (s *Snapshot) In(id string) []*Edge  { return s.in[id] }

// FromResponse builds a report snapshot from mantis-engine's internal
// snapshot (see internal/snapshotapi). mantis-report has no Kubernetes client
// of its own: everything it describes comes from the engine.
func FromResponse(r snapshotapi.Response) (*Snapshot, error) {
	var dto struct {
		Meta struct {
			Context       string   `json:"context"`
			Server        string   `json:"server"`
			Version       string   `json:"version"`
			ClusterID     string   `json:"clusterId"`
			Revision      uint64   `json:"revision"`
			NamespaceList []string `json:"namespaceList"`
			Skipped       []string `json:"skipped"`
		} `json:"meta"`
		Nodes []struct {
			ID         string   `json:"id"`
			Kind       string   `json:"kind"`
			Name       string   `json:"name"`
			NS         string   `json:"ns"`
			Status     string   `json:"status"`
			Resolved   bool     `json:"resolved"`
			Unverified bool     `json:"unverified"`
			Attributes []string `json:"attributes"`
		} `json:"nodes"`
		Edges []struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Type     string `json:"type"`
			Category string `json:"category"`
			Basis    string `json:"basis"`
			Evidence string `json:"evidence"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(r.Graph, &dto); err != nil {
		return nil, fmt.Errorf("decode engine graph: %w", err)
	}
	g := Graph{Context: dto.Meta.Context, Server: dto.Meta.Server, ClusterID: dto.Meta.ClusterID, Version: dto.Meta.Version,
		SyncMode: r.SyncMode, Revision: r.Revision, Namespaces: dto.Meta.NamespaceList, Skipped: dto.Meta.Skipped}
	for _, n := range dto.Nodes {
		g.Nodes = append(g.Nodes, Node{ID: n.ID, Kind: n.Kind, Name: n.Name, Namespace: n.NS, Status: n.Status, Resolved: n.Resolved, Unverified: n.Unverified, Attributes: n.Attributes})
	}
	for _, e := range dto.Edges {
		g.Edges = append(g.Edges, Edge{From: e.From, To: e.To, Type: e.Type, Category: e.Category, Basis: e.Basis, Evidence: e.Evidence})
	}
	o := r.Objects
	s := &Snapshot{Graph: g, TakenAt: r.TakenAt.UTC(),
		Namespaces: o.Namespaces, Pods: o.Pods, Deployments: o.Deployments, ReplicaSets: o.ReplicaSets, StatefulSets: o.StatefulSets,
		DaemonSets: o.DaemonSets, Jobs: o.Jobs, CronJobs: o.CronJobs, Services: o.Services, Ingresses: o.Ingresses,
		NetworkPolicies: o.NetworkPolicies, ConfigMaps: o.ConfigMaps, Secrets: o.Secrets, PVCs: o.PVCs, PVs: o.PVs,
		StorageClasses: o.StorageClasses, Nodes: o.Nodes, ServiceAccounts: o.ServiceAccounts, Roles: o.Roles,
		RoleBindings: o.RoleBindings, ClusterRoles: o.ClusterRoles, ClusterRoleBindings: o.ClusterRoleBindings,
		HPAs: o.HPAs, ResourceQuotas: o.ResourceQuotas, LimitRanges: o.LimitRanges}
	// Defence in depth: the engine never sends Secret values, but make sure.
	for i := range s.Secrets {
		s.Secrets[i].Data, s.Secrets[i].StringData = nil, nil
	}
	if s.TakenAt.IsZero() {
		s.TakenAt = time.Now().UTC()
	}
	s.NotReadable = mergeStrings(r.NotReadable, g.Skipped)
	s.index()
	s.sortAll()
	return s, nil
}

func (s *Snapshot) index() {
	s.byID = make(map[string]*Node, len(s.Graph.Nodes))
	s.edges = make(map[string]*Edge, len(s.Graph.Edges))
	s.out, s.in = map[string][]*Edge{}, map[string][]*Edge{}
	for i := range s.Graph.Nodes {
		n := &s.Graph.Nodes[i]
		s.byID[n.ID] = n
	}
	for i := range s.Graph.Edges {
		e := &s.Graph.Edges[i]
		s.edges[e.Key()] = e
		s.out[e.From] = append(s.out[e.From], e)
		s.in[e.To] = append(s.in[e.To], e)
	}
}

func (s *Snapshot) sortAll() {
	byNsName := func(a, b metav1.ObjectMeta) bool {
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	}
	sort.Slice(s.Pods, func(i, j int) bool { return byNsName(s.Pods[i].ObjectMeta, s.Pods[j].ObjectMeta) })
	sort.Slice(s.Deployments, func(i, j int) bool { return byNsName(s.Deployments[i].ObjectMeta, s.Deployments[j].ObjectMeta) })
	sort.Slice(s.ReplicaSets, func(i, j int) bool { return byNsName(s.ReplicaSets[i].ObjectMeta, s.ReplicaSets[j].ObjectMeta) })
	sort.Slice(s.StatefulSets, func(i, j int) bool { return byNsName(s.StatefulSets[i].ObjectMeta, s.StatefulSets[j].ObjectMeta) })
	sort.Slice(s.DaemonSets, func(i, j int) bool { return byNsName(s.DaemonSets[i].ObjectMeta, s.DaemonSets[j].ObjectMeta) })
	sort.Slice(s.Jobs, func(i, j int) bool { return byNsName(s.Jobs[i].ObjectMeta, s.Jobs[j].ObjectMeta) })
	sort.Slice(s.CronJobs, func(i, j int) bool { return byNsName(s.CronJobs[i].ObjectMeta, s.CronJobs[j].ObjectMeta) })
	sort.Slice(s.Services, func(i, j int) bool { return byNsName(s.Services[i].ObjectMeta, s.Services[j].ObjectMeta) })
	sort.Slice(s.Ingresses, func(i, j int) bool { return byNsName(s.Ingresses[i].ObjectMeta, s.Ingresses[j].ObjectMeta) })
	sort.Slice(s.NetworkPolicies, func(i, j int) bool { return byNsName(s.NetworkPolicies[i].ObjectMeta, s.NetworkPolicies[j].ObjectMeta) })
	sort.Slice(s.ConfigMaps, func(i, j int) bool { return byNsName(s.ConfigMaps[i].ObjectMeta, s.ConfigMaps[j].ObjectMeta) })
	sort.Slice(s.Secrets, func(i, j int) bool { return byNsName(s.Secrets[i].ObjectMeta, s.Secrets[j].ObjectMeta) })
	sort.Slice(s.PVCs, func(i, j int) bool { return byNsName(s.PVCs[i].ObjectMeta, s.PVCs[j].ObjectMeta) })
	sort.Slice(s.PVs, func(i, j int) bool { return s.PVs[i].Name < s.PVs[j].Name })
	sort.Slice(s.StorageClasses, func(i, j int) bool { return s.StorageClasses[i].Name < s.StorageClasses[j].Name })
	sort.Slice(s.Nodes, func(i, j int) bool { return s.Nodes[i].Name < s.Nodes[j].Name })
	sort.Slice(s.ServiceAccounts, func(i, j int) bool { return byNsName(s.ServiceAccounts[i].ObjectMeta, s.ServiceAccounts[j].ObjectMeta) })
	sort.Slice(s.Roles, func(i, j int) bool { return byNsName(s.Roles[i].ObjectMeta, s.Roles[j].ObjectMeta) })
	sort.Slice(s.RoleBindings, func(i, j int) bool { return byNsName(s.RoleBindings[i].ObjectMeta, s.RoleBindings[j].ObjectMeta) })
	sort.Slice(s.ClusterRoles, func(i, j int) bool { return s.ClusterRoles[i].Name < s.ClusterRoles[j].Name })
	sort.Slice(s.ClusterRoleBindings, func(i, j int) bool { return s.ClusterRoleBindings[i].Name < s.ClusterRoleBindings[j].Name })
	sort.Slice(s.HPAs, func(i, j int) bool { return byNsName(s.HPAs[i].ObjectMeta, s.HPAs[j].ObjectMeta) })
	sort.Slice(s.ResourceQuotas, func(i, j int) bool { return byNsName(s.ResourceQuotas[i].ObjectMeta, s.ResourceQuotas[j].ObjectMeta) })
	sort.Slice(s.LimitRanges, func(i, j int) bool { return byNsName(s.LimitRanges[i].ObjectMeta, s.LimitRanges[j].ObjectMeta) })
	sort.Slice(s.Namespaces, func(i, j int) bool { return s.Namespaces[i].Name < s.Namespaces[j].Name })
}

func mergeStrings(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range append(append([]string{}, a...), b...) {
		set[x] = true
	}
	out := make([]string, 0, len(set))
	for x := range set {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
