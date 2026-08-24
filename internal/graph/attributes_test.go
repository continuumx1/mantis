package graph

import "testing"

func TestIsHiddenNamespace(t *testing.T) {
	cases := map[string]bool{
		"kube-node-lease": true,
		"kube-system":     false,
		"default":         false,
		"kube-public":     false,
	}
	for name, want := range cases {
		if got := IsHiddenNamespace(name); got != want {
			t.Errorf("IsHiddenNamespace(%q) = %v, want %v", name, got, want)
		}
	}
}
