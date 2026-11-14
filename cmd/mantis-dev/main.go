// Command mantis-dev runs the real Mantis engine and UI against a synthetic,
// in-memory cluster — no Kubernetes required. It exists for local UI work,
// demos and performance testing at sizes (1,000-10,000+ resources) that are
// impractical to stand up for real.
//
//	go run ./cmd/mantis-dev -resources 5000            # serve on :8090
//	go run ./cmd/mantis-dev -resources 2000 -churn 5s  # with live changes
//	go run ./cmd/mantis-dev -resources 10000 -dump g.json  # write a graph and exit
//	go run ./cmd/mantis-dev -kube                      # your current kubeconfig context instead
//
// The engine is the production engine package, reading a fake clientset that
// holds synth.Generate's objects, so every relationship in the graph comes
// from the real resolvers. There is no login (it never talks to a real
// cluster); do not expose it beyond localhost.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/continuumx1/mantis/internal/engine"
	"github.com/continuumx1/mantis/internal/graph"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
	"github.com/continuumx1/mantis/internal/synth"
	"github.com/continuumx1/mantis/internal/web"
)

func main() {
	resources := flag.Int("resources", 1500, "approximate number of synthetic Kubernetes resources")
	seed := flag.Int64("seed", 1, "generator seed (same seed, same cluster)")
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	uiDir := flag.String("ui-dir", "", "serve the UI from this directory instead of the embedded copy (live edits)")
	churn := flag.Duration("churn", 0, "if set, mutate the cluster this often (reschedules, scale-ups, crashes)")
	syncEvery := flag.Duration("sync", 3*time.Second, "engine sync interval")
	dump := flag.String("dump", "", "write the built graph (GraphDTO JSON) to this file and exit")
	syncMode := flag.String("sync-mode", "watch", "watch (list once, then follow changes) or poll (re-list every -sync)")
	kube := flag.Bool("kube", false, "read your current kubeconfig context instead of a synthetic cluster (read-only, like the real engine)")
	flag.Parse()

	if *kube {
		client, err := mantiskube.NewClient()
		if err != nil {
			fail(err)
		}
		srv := engine.New(client, false, *syncEvery)
		srv.SetSyncMode(*syncMode)
		serve(srv, *uiDir, *addr)
		return
	}

	objs := synth.Generate(synth.Options{Resources: *resources, Seed: *seed})
	cs := fake.NewSimpleClientset(objs...)
	slog.Info("synthetic cluster", "objects", len(objs))

	if *dump != "" {
		gctx, skipped, err := graph.BuildClusterGraph(context.Background(), cs, nil, false)
		if err != nil {
			fail(err)
		}
		dto := engine.FromContext(gctx, engine.MetaDTO{Context: fmt.Sprintf("synthetic-%d", *resources), Skipped: skipped, Revision: 1})
		f, err := os.Create(*dump)
		if err != nil {
			fail(err)
		}
		if err := json.NewEncoder(f).Encode(dto); err != nil {
			fail(err)
		}
		f.Close()
		slog.Info("wrote graph", "file", *dump, "nodes", len(dto.Nodes), "edges", len(dto.Edges))
		return
	}

	client := &mantiskube.Client{Clientset: cs, Context: fmt.Sprintf("synthetic-%d", *resources), Server: "https://synthetic.invalid"}
	if *churn > 0 {
		go churnLoop(cs, *churn, *seed)
	}
	srv := engine.New(client, false, *syncEvery)
	srv.SetSyncMode(*syncMode)
	serve(srv, *uiDir, *addr)
}

// serve runs the engine's sync loop and serves its API plus the UI, with no
// login (mantis-dev is a localhost tool; do not expose it).
func serve(srv *engine.Server, uiDir, addr string) {
	srv.Start(context.Background())
	var static http.Handler
	if uiDir != "" {
		static = web.NoCache(http.FileServer(http.Dir(uiDir)))
	} else {
		h, err := web.StaticHandler()
		if err != nil {
			fail(err)
		}
		static = h
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Handler())
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusSeeOther) })
	mux.Handle("/", static)
	slog.Info("mantis-dev listening", "url", "http://"+addr+"/")
	if err := http.ListenAndServe(addr, mux); err != nil {
		fail(err)
	}
}

// churnLoop makes small, realistic changes on a timer so incremental graph
// updates (revisions, deltas, stable layout under change) can be observed:
// a Pod rescheduled onto another Node, a replica added, a replica crashing.
func churnLoop(cs *fake.Clientset, every time.Duration, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	ctx := context.Background()
	nodes, _ := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	for range time.Tick(every) {
		pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil || len(pods.Items) == 0 || len(nodes.Items) == 0 {
			continue
		}
		p := pods.Items[rng.Intn(len(pods.Items))]
		switch rng.Intn(3) {
		case 0: // reschedule: same Pod identity, new Node
			_ = cs.CoreV1().Pods(p.Namespace).Delete(ctx, p.Name, metav1.DeleteOptions{})
			np := p.DeepCopy()
			np.ResourceVersion = ""
			np.Spec.NodeName = nodes.Items[rng.Intn(len(nodes.Items))].Name
			_, _ = cs.CoreV1().Pods(p.Namespace).Create(ctx, np, metav1.CreateOptions{})
			slog.Info("churn", "event", "reschedule", "pod", p.Namespace+"/"+p.Name, "node", np.Spec.NodeName)
		case 1: // scale up: a sibling replica
			np := p.DeepCopy()
			np.ResourceVersion = ""
			np.UID = ""
			np.Name = fmt.Sprintf("%s-x%04d", p.Name, rng.Intn(10000))
			_, _ = cs.CoreV1().Pods(p.Namespace).Create(ctx, np, metav1.CreateOptions{})
			slog.Info("churn", "event", "scale-up", "pod", p.Namespace+"/"+np.Name)
		case 2: // crash
			np := p.DeepCopy()
			for i := range np.Status.ContainerStatuses {
				np.Status.ContainerStatuses[i].Ready = false
				np.Status.ContainerStatuses[i].RestartCount++
				np.Status.ContainerStatuses[i].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
			}
			_, _ = cs.CoreV1().Pods(p.Namespace).UpdateStatus(ctx, np, metav1.UpdateOptions{})
			slog.Info("churn", "event", "crash", "pod", p.Namespace+"/"+p.Name)
		}
	}
}

func fail(err error) {
	slog.Error("mantis-dev", "error", err.Error())
	os.Exit(1)
}
