package engine

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// These endpoints back the application pages around the topology (dashboard,
// events, capacity). Like everything else in the engine they are read-only,
// and like /api/graph they never put an unbounded live read on the request
// path: each result is cached briefly, so a dashboard left open, or many
// browsers, cost the API server one List per TTL rather than one per request.

const (
	eventsTTL   = 10 * time.Second
	capacityTTL = 15 * time.Second
	maxEvents   = 500
)

// ttlCache holds one computed value for a fixed time.
type ttlCache[T any] struct {
	mu   sync.Mutex
	at   time.Time
	val  T
	err  error
	have bool
}

func (c *ttlCache[T]) get(ttl time.Duration, compute func() (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.have && time.Since(c.at) < ttl {
		return c.val, c.err
	}
	c.val, c.err = compute()
	c.at, c.have = time.Now(), true
	return c.val, c.err
}

// EventDTO is one Kubernetes Event, flattened for display.
type EventDTO struct {
	Type      string `json:"type"` // Normal | Warning
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"ns"`
	// ObjectID is the involved object's graph ID ("<ns>/<Kind>/<name>"), so
	// the UI can link an event to the resource it is about.
	ObjectID string `json:"objectId"`
	Count    int32  `json:"count"`
	Last     string `json:"last"` // RFC 3339
	Source   string `json:"source,omitempty"`
}

// EventsDTO is the /api/events payload.
type EventsDTO struct {
	Events    []EventDTO `json:"events"`
	Total     int        `json:"total"`     // events in the cluster (before the cap)
	Warnings  int        `json:"warnings"`  // Warning events in the cluster
	Forbidden bool       `json:"forbidden"` // RBAC does not allow listing events
}

// handleEvents serves /api/events?type=Warning&ns=&kind=&name=&limit= — the
// most recent Kubernetes Events, newest first.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	all, err := s.events.get(eventsTTL, func() (EventsDTO, error) { return s.readEvents(r.Context()) })
	if err != nil {
		http.Error(w, "list events: "+err.Error(), http.StatusBadGateway)
		return
	}
	q := r.URL.Query()
	limit := 200
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= maxEvents {
		limit = v
	}
	out := EventsDTO{Total: all.Total, Warnings: all.Warnings, Forbidden: all.Forbidden, Events: []EventDTO{}}
	for _, e := range all.Events {
		if t := q.Get("type"); t != "" && e.Type != t {
			continue
		}
		if ns := q.Get("ns"); ns != "" && e.Namespace != ns {
			continue
		}
		if k := q.Get("kind"); k != "" && e.Kind != k {
			continue
		}
		if n := q.Get("name"); n != "" && e.Name != n {
			continue
		}
		out.Events = append(out.Events, e)
		if len(out.Events) >= limit {
			break
		}
	}
	writeJSON(w, r, out)
}

func (s *Server) readEvents(ctx context.Context) (EventsDTO, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), getTimeout)
	defer cancel()
	list, err := s.client.Clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if isForbidden(err) {
			return EventsDTO{Forbidden: true, Events: []EventDTO{}}, nil
		}
		slog.Error("events", "event", "kubernetes_api_failure", "error", err.Error())
		return EventsDTO{}, err
	}
	out := EventsDTO{Total: len(list.Items)}
	evs := make([]EventDTO, 0, len(list.Items))
	for i := range list.Items {
		e := &list.Items[i]
		if e.Type == corev1.EventTypeWarning {
			out.Warnings++
		}
		last := e.LastTimestamp.Time
		if last.IsZero() {
			last = e.EventTime.Time
		}
		if last.IsZero() {
			last = e.CreationTimestamp.Time
		}
		obj := e.InvolvedObject
		evs = append(evs, EventDTO{
			Type: e.Type, Reason: e.Reason, Message: e.Message,
			Kind: obj.Kind, Name: obj.Name, Namespace: obj.Namespace,
			ObjectID: obj.Namespace + "/" + obj.Kind + "/" + obj.Name,
			Count:    max(e.Count, 1), Last: last.UTC().Format(time.RFC3339),
			Source: e.Source.Component,
		})
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].Last > evs[j].Last })
	if len(evs) > maxEvents {
		evs = evs[:maxEvents]
	}
	out.Events = evs
	return out, nil
}

// CapacityDTO is the /api/cluster/capacity payload. Allocatable and capacity
// come from Node status and are always real. Usage comes only from the
// metrics.k8s.io API (metrics-server); when that API is absent, Metrics is
// false and usage is omitted — Mantis never estimates or simulates it.
type CapacityDTO struct {
	Nodes          int    `json:"nodes"`
	ReadyNodes     int    `json:"readyNodes"`
	CPUAllocatable int64  `json:"cpuAllocatableMilli"`
	CPUCapacity    int64  `json:"cpuCapacityMilli"`
	MemAllocatable int64  `json:"memAllocatableBytes"`
	MemCapacity    int64  `json:"memCapacityBytes"`
	PodCapacity    int64  `json:"podCapacity"`
	Metrics        bool   `json:"metrics"`
	MetricsReason  string `json:"metricsReason,omitempty"`
	CPUUsage       int64  `json:"cpuUsageMilli,omitempty"`
	MemUsage       int64  `json:"memUsageBytes,omitempty"`
	PerNode        []struct {
		Name           string `json:"name"`
		Ready          bool   `json:"ready"`
		CPUAllocatable int64  `json:"cpuAllocatableMilli"`
		MemAllocatable int64  `json:"memAllocatableBytes"`
		CPUUsage       int64  `json:"cpuUsageMilli,omitempty"`
		MemUsage       int64  `json:"memUsageBytes,omitempty"`
	} `json:"perNode"`
}

var nodeMetricsGVR = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}

func (s *Server) handleCapacity(w http.ResponseWriter, r *http.Request) {
	c, err := s.capacity.get(capacityTTL, func() (CapacityDTO, error) { return s.readCapacity(r.Context()) })
	if err != nil {
		http.Error(w, "read capacity: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, r, c)
}

func (s *Server) readCapacity(ctx context.Context) (CapacityDTO, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), getTimeout)
	defer cancel()
	nodes, err := s.readClient().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return CapacityDTO{}, err
	}
	var out CapacityDTO
	idx := map[string]int{}
	for i := range nodes.Items {
		n := &nodes.Items[i]
		ready := false
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		out.Nodes++
		if ready {
			out.ReadyNodes++
		}
		cpuA, memA := milli(n.Status.Allocatable, corev1.ResourceCPU), bytesOf(n.Status.Allocatable, corev1.ResourceMemory)
		out.CPUAllocatable += cpuA
		out.MemAllocatable += memA
		out.CPUCapacity += milli(n.Status.Capacity, corev1.ResourceCPU)
		out.MemCapacity += bytesOf(n.Status.Capacity, corev1.ResourceMemory)
		if p, ok := n.Status.Allocatable[corev1.ResourcePods]; ok {
			out.PodCapacity += p.Value()
		}
		idx[n.Name] = len(out.PerNode)
		out.PerNode = append(out.PerNode, struct {
			Name           string `json:"name"`
			Ready          bool   `json:"ready"`
			CPUAllocatable int64  `json:"cpuAllocatableMilli"`
			MemAllocatable int64  `json:"memAllocatableBytes"`
			CPUUsage       int64  `json:"cpuUsageMilli,omitempty"`
			MemUsage       int64  `json:"memUsageBytes,omitempty"`
		}{Name: n.Name, Ready: ready, CPUAllocatable: cpuA, MemAllocatable: memA})
	}
	sort.Slice(out.PerNode, func(i, j int) bool { return out.PerNode[i].Name < out.PerNode[j].Name })
	for i, p := range out.PerNode {
		idx[p.Name] = i
	}

	if s.client.Dynamic == nil {
		out.MetricsReason = "dynamic client unavailable"
		return out, nil
	}
	list, err := s.client.Dynamic.Resource(nodeMetricsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		switch {
		case isForbidden(err):
			out.MetricsReason = "reading metrics.k8s.io is forbidden by RBAC"
		default:
			out.MetricsReason = "the metrics API (metrics-server) is not installed or not reachable"
		}
		return out, nil
	}
	if len(list.Items) == 0 {
		// metrics-server answers but has not reported any node yet (it was
		// just installed or restarted): there is no usage to show.
		out.MetricsReason = "metrics-server has not reported node metrics yet"
		return out, nil
	}
	out.Metrics = true
	for _, item := range list.Items {
		usage, _, _ := unstructuredStringMap(item.Object, "usage")
		cpu, mem := parseQty(usage["cpu"], true), parseQty(usage["memory"], false)
		out.CPUUsage += cpu
		out.MemUsage += mem
		if i, ok := idx[item.GetName()]; ok {
			out.PerNode[i].CPUUsage, out.PerNode[i].MemUsage = cpu, mem
		}
	}
	return out, nil
}

func milli(rl corev1.ResourceList, name corev1.ResourceName) int64 {
	if q, ok := rl[name]; ok {
		return q.MilliValue()
	}
	return 0
}

func bytesOf(rl corev1.ResourceList, name corev1.ResourceName) int64 {
	if q, ok := rl[name]; ok {
		return q.Value()
	}
	return 0
}

func parseQty(s string, asMilli bool) int64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	if asMilli {
		return q.MilliValue()
	}
	return q.Value()
}

func unstructuredStringMap(obj map[string]any, field string) (map[string]string, bool, error) {
	raw, ok := obj[field].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out, true, nil
}

func isForbidden(err error) bool { return apierrors.IsForbidden(err) }
