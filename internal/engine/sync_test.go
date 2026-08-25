package engine

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func namespaceObj(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

// syncOnce must refuse to do any work at all while another call is already
// in flight — proven here by pre-setting the guard by hand and checking
// nothing ran, rather than racing two real goroutines against each other.
func TestSyncOnce_SkipsWhenAlreadySyncing(t *testing.T) {
	s := newTestServerWithClient(fake.NewSimpleClientset(namespaceObj("ns-a")))
	s.syncing.Store(true) // simulate a pass already in flight

	s.syncOnce(context.Background())

	if got := s.progress.snapshot(); got.StartedAt != nil {
		t.Errorf("syncOnce did work despite syncing already being true: progress = %+v", got)
	}
	if _, ok := s.cache.get(); ok {
		t.Error("syncOnce published a snapshot despite syncing already being true")
	}
}

// The guard must release once a pass finishes, so the next scheduled tick
// (or, before Commit 4's overlap guard existed, the very next call) isn't
// permanently locked out.
func TestSyncOnce_ReleasesGuardAfterCompletion(t *testing.T) {
	s := newTestServerWithClient(fake.NewSimpleClientset(namespaceObj("ns-a")))

	s.syncOnce(context.Background())
	if s.syncing.Load() {
		t.Fatal("syncing flag left set after syncOnce returned")
	}
	first := s.progress.snapshot().LastSyncAt
	if first == nil {
		t.Fatal("expected LastSyncAt to be set after a completed pass")
	}

	time.Sleep(time.Millisecond) // force a distinguishable timestamp
	s.syncOnce(context.Background())
	second := s.progress.snapshot().LastSyncAt
	if second == nil || !second.After(*first) {
		t.Errorf("second syncOnce call did not run (guard not released?): first=%v second=%v", first, second)
	}
}

// A syncOnce whose deadline has already passed must fail with the sync it
// attempted, not silently succeed with an empty/partial graph — and must do
// so via context cancellation, not a timer race, so the test is deterministic.
func TestSyncOnce_TimesOutOnAnAlreadyExpiredContext(t *testing.T) {
	s := newTestServerWithClient(fake.NewSimpleClientset(namespaceObj("ns-a")))

	ctx, cancel := context.WithTimeout(context.Background(), 0) // already expired
	defer cancel()
	s.syncOnce(ctx)

	got := s.progress.snapshot()
	if got.LastError == "" {
		t.Error("expected LastError to be set for a sync run against an already-expired context")
	}
	if got.Running {
		t.Error("progress left Running=true after syncOnce returned")
	}
}
