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
