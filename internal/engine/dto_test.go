package engine

import "testing"

func TestNodeStatus(t *testing.T) {
	cases := []struct {
		name  string
		attrs []string
		want  string
	}{
		{"ready is ok", []string{"status: Ready"}, "ok"},
		{"not ready is crit", []string{"status: NotReady"}, "crit"},
		{"unknown is warn, not folded into ok or crit", []string{"status: Unknown"}, "warn"},
		{"no attributes at all carries no ring", nil, ""},
		{"unrecognized first attribute carries no ring", []string{"roles: control-plane"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nodeStatus(c.attrs); got != c.want {
				t.Errorf("nodeStatus(%v) = %q, want %q", c.attrs, got, c.want)
			}
		})
	}
}

func TestStatusFor_DispatchesByKind(t *testing.T) {
	if got := statusFor("Node", []string{"status: NotReady"}); got != "crit" {
		t.Errorf(`statusFor("Node", ...) = %q, want "crit"`, got)
	}
	if got := statusFor("Pod", []string{"Running · 1/1"}); got != "ok" {
		t.Errorf(`statusFor("Pod", ...) = %q, want "ok"`, got)
	}
	if got := statusFor("Service", []string{"type: ClusterIP"}); got != "" {
		t.Errorf(`statusFor("Service", ...) = %q, want "" (no health model for this kind)`, got)
	}
}
