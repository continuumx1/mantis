package graph

import "testing"

func TestContext_FromAndExistence(t *testing.T) {
	subject := ResourceRef{Kind: "Ingress", Name: "web", Namespace: "default"}
	present := ResourceRef{Kind: "Service", Name: "api", Namespace: "default"}
	missing := ResourceRef{Kind: "Service", Name: "gone", Namespace: "default"}
	unchecked := ResourceRef{Kind: "Service", Name: "unverified", Namespace: "default"}

	relations := []Relation{
		{From: subject, Type: RoutesTo, To: present},
		{From: subject, Type: RoutesTo, To: missing},
		{From: subject, Type: RoutesTo, To: unchecked},
	}
	existence := map[ResourceRef]bool{
		present: true,
		missing: false,
		// unchecked deliberately absent
	}

	c := New(subject, relations, existence)

	if got := c.From(subject, RoutesTo); len(got) != 3 {
		t.Errorf("expected 3 routes-to relations, got %d", len(got))
	}
	if got := c.From(subject, Selects); len(got) != 0 {
		t.Errorf("expected 0 selects relations, got %d", len(got))
	}

	if resolved, checked := c.Existence(present); !checked || !resolved {
		t.Errorf("present: want (resolved=true, checked=true), got (%v, %v)", resolved, checked)
	}
	if resolved, checked := c.Existence(missing); !checked || resolved {
		t.Errorf("missing: want (resolved=false, checked=true), got (%v, %v)", resolved, checked)
	}
	if _, checked := c.Existence(unchecked); checked {
		t.Errorf("unchecked: want checked=false, got checked=true")
	}
}

func TestContext_Into(t *testing.T) {
	svc := ResourceRef{Kind: "Service", Name: "web", Namespace: "default"}
	pod := ResourceRef{Kind: "Pod", Name: "web-1", Namespace: "default"}
	other := ResourceRef{Kind: "Pod", Name: "web-2", Namespace: "default"}

	c := New(svc, []Relation{
		{From: svc, Type: Selects, To: pod},
		{From: svc, Type: Selects, To: other},
		{From: svc, Type: Serves, To: pod},
	}, nil)

	if got := c.Into(pod, Selects); len(got) != 1 || got[0].From != svc {
		t.Errorf("Into(pod, Selects) = %+v, want one edge from Service/web", got)
	}
	if got := c.Into(pod, Serves); len(got) != 1 {
		t.Errorf("Into(pod, Serves) = %+v, want one serves edge", got)
	}
	if got := c.Into(other, Serves); len(got) != 0 {
		t.Errorf("Into(other, Serves) = %+v, want none", got)
	}
}
