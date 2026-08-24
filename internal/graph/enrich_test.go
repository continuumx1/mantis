package graph

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nodeReadiness returns the "status: ..." attribute nodeAttributes produces,
// stripped of its "status: " prefix, for a Node with the given NodeReady
// condition. hasCondition=false omits the condition entirely, simulating a
// Node whose kubelet has never reported one.
func nodeReadiness(t *testing.T, status corev1.ConditionStatus, hasCondition bool) string {
	t.Helper()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	if hasCondition {
		node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}}
	}
	attrs := nodeAttributes(node)
	if len(attrs) == 0 {
		t.Fatal("nodeAttributes returned no attributes")
	}
	const prefix = "status: "
	if len(attrs[0]) < len(prefix) || attrs[0][:len(prefix)] != prefix {
		t.Fatalf("attrs[0] = %q, want it to start with %q", attrs[0], prefix)
	}
	return attrs[0][len(prefix):]
}

func TestNodeAttributes_Readiness(t *testing.T) {
	cases := []struct {
		name         string
		status       corev1.ConditionStatus
		hasCondition bool
		want         string
	}{
		{"condition true is Ready", corev1.ConditionTrue, true, "Ready"},
		{"condition false is NotReady, not Unknown", corev1.ConditionFalse, true, "NotReady"},
		{"condition unknown is Unknown, not NotReady", corev1.ConditionUnknown, true, "Unknown"},
		{"missing condition is Unknown, never assumed NotReady", "", false, "Unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nodeReadiness(t, c.status, c.hasCondition); got != c.want {
				t.Errorf("readiness = %q, want %q", got, c.want)
			}
		})
	}
}
