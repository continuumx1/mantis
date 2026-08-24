package engine

import "testing"

func TestSnapshotCache_GetBeforePublish(t *testing.T) {
	c := newSnapshotCache()
	if _, ok := c.get(); ok {
		t.Error("get() on a fresh cache: ok = true, want false")
	}
	select {
	case <-c.ready:
		t.Error("ready channel closed before any publish")
	default:
	}
}

func TestSnapshotCache_PublishThenGet(t *testing.T) {
	c := newSnapshotCache()
	want := GraphDTO{Meta: MetaDTO{Context: "test"}}
	c.publish(want)

	got, ok := c.get()
	if !ok {
		t.Fatal("get() after publish: ok = false, want true")
	}
	if got.Meta.Context != "test" {
		t.Errorf("get() = %+v, want Meta.Context = %q", got, "test")
	}
	select {
	case <-c.ready:
	default:
		t.Error("ready channel not closed after publish")
	}
}

func TestSnapshotCache_LaterPublishOverwrites(t *testing.T) {
	c := newSnapshotCache()
	c.publish(GraphDTO{Meta: MetaDTO{Context: "first"}})
	c.publish(GraphDTO{Meta: MetaDTO{Context: "second"}})

	got, ok := c.get()
	if !ok || got.Meta.Context != "second" {
		t.Errorf("get() = %+v (ok=%v), want Meta.Context = %q", got, ok, "second")
	}
}
