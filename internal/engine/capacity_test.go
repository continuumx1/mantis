package engine

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
)

func capacityServer(t *testing.T, metrics ...*unstructured.Unstructured) *Server {
	t.Helper()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4"), corev1.ResourceMemory: resource.MustParse("8Gi")},
		},
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{nodeMetricsGVR: "NodeMetricsList"})
	for _, m := range metrics {
		if _, err := dyn.Resource(nodeMetricsGVR).Create(context.Background(), m, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return &Server{client: &mantiskube.Client{Clientset: fake.NewSimpleClientset(node), Dynamic: dyn}}
}

// metrics-server answering with no node metrics yet must not be reported as
// usage of zero (the dashboard showed "NaN cores used").
func TestCapacity_MetricsServerWithoutNodeMetrics(t *testing.T) {
	c, err := capacityServer(t).readCapacity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.Metrics || c.MetricsReason == "" {
		t.Fatalf("want metrics unavailable with a reason, got metrics=%v reason=%q", c.Metrics, c.MetricsReason)
	}
}

func TestCapacity_SumsNodeMetrics(t *testing.T) {
	m := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "metrics.k8s.io/v1beta1", "kind": "NodeMetrics",
		"metadata": map[string]interface{}{"name": "worker-1"},
		"usage":    map[string]interface{}{"cpu": "1500m", "memory": "2Gi"},
	}}
	c, err := capacityServer(t, m).readCapacity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !c.Metrics || c.CPUUsage != 1500 || c.MemUsage != 2<<30 || c.PerNode[0].CPUUsage != 1500 {
		t.Fatalf("usage not summed: %+v", c)
	}
}
