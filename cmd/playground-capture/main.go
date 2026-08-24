// Command playground-capture is a dev-only tool (never built into the shipped
// mantis-engine/mantis-web images — see build/Dockerfile.*, which only build
// ./cmd/mantis-engine and ./cmd/mantis-web) that generates the static fixture
// data behind the Mantis Playground (internal/web/ui/playground.html).
//
// It points at whatever cluster the current kubeconfig context resolves to,
// builds the real relationship graph for one namespace using the same
// graph/engine code the product ships, and writes:
//
//	<out>/graph.json            — the exact GraphDTO shape /api/graph serves
//	<out>/resources/<id>.yaml   — one manifest per resolved, non-Secret node
//
// This is how the Playground gets to reuse the real mantis-web frontend
// against real captured data instead of hand-written fake JSON: the fixtures
// are a genuine snapshot of a real cluster running the scenario manifests in
// internal/web/ui/playground/manifests/.
//
// Usage:
//
//	go run ./cmd/playground-capture \
//	  -namespace shop-frontend \
//	  -context-label "playground · web application (sample data)" \
//	  -out internal/web/ui/playground/data/web-app
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/continuumx1/mantis/internal/engine"
	"github.com/continuumx1/mantis/internal/graph"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
)

const captureTimeout = 30 * time.Second

// kindGVR mirrors internal/engine/resource.go's unexported kindGVR — duplicated
// here rather than imported because that map (and the handler around it) is
// deliberately private to the engine package; this tool only ever needs the
// handful of kinds the playground scenarios actually use.
var kindGVR = map[string]schema.GroupVersionResource{
	"Pod":                     {Group: "", Version: "v1", Resource: "pods"},
	"Service":                 {Group: "", Version: "v1", Resource: "services"},
	"ConfigMap":               {Group: "", Version: "v1", Resource: "configmaps"},
	"PersistentVolumeClaim":   {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
	"PersistentVolume":        {Group: "", Version: "v1", Resource: "persistentvolumes"},
	"Node":                    {Group: "", Version: "v1", Resource: "nodes"},
	"Deployment":              {Group: "apps", Version: "v1", Resource: "deployments"},
	"ReplicaSet":              {Group: "apps", Version: "v1", Resource: "replicasets"},
	"StatefulSet":             {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":               {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"Ingress":                 {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	"HorizontalPodAutoscaler": {Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
}

func main() {
	namespace := flag.String("namespace", "", "namespace to capture (required)")
	contextLabel := flag.String("context-label", "", "friendly label shown as the cluster context in the Playground header (required)")
	serverLabel := flag.String("server-label", "sample data — not a live cluster", "friendly label shown in place of a real API server address")
	out := flag.String("out", "", "output directory for this scenario's graph.json + resources/ (required)")
	showAll := flag.Bool("show-all", false, "include system-managed noise, same meaning as MANTIS_SHOW_ALL")
	flag.Parse()

	if *namespace == "" || *contextLabel == "" || *out == "" {
		log.Fatal("playground-capture: -namespace, -context-label, and -out are all required")
	}

	client, err := mantiskube.NewClient()
	if err != nil {
		log.Fatalf("playground-capture: connect to Kubernetes: %v", err)
	}
	log.Printf("playground-capture: capturing namespace %q from context %q (%s)", *namespace, client.Context, client.Server)

	ctx, cancel := context.WithTimeout(context.Background(), captureTimeout)
	defer cancel()

	gctx, skipped, err := graph.BuildNamespaceGraph(ctx, client.Clientset, client.Dynamic, *namespace, *showAll)
	if err != nil {
		log.Fatalf("playground-capture: build namespace graph: %v", err)
	}
	if len(skipped) > 0 {
		log.Printf("playground-capture: warning — RBAC skipped kinds: %v", skipped)
	}

	meta := engine.MetaDTO{
		Context:       *contextLabel,
		Server:        *serverLabel,
		NamespaceList: []string{*namespace},
		Namespaces:    1,
	}
	if v, err := client.Clientset.Discovery().ServerVersion(); err == nil {
		meta.Version = v.GitVersion
	}

	dto := engine.FromContext(gctx, meta)
	pruneForeignClusterScoped(&dto, *namespace)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("playground-capture: mkdir %s: %v", *out, err)
	}
	if err := os.MkdirAll(filepath.Join(*out, "resources"), 0o755); err != nil {
		log.Fatalf("playground-capture: mkdir resources: %v", err)
	}

	graphPath := filepath.Join(*out, "graph.json")
	f, err := os.Create(graphPath)
	if err != nil {
		log.Fatalf("playground-capture: create %s: %v", graphPath, err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(dto); err != nil {
		f.Close()
		log.Fatalf("playground-capture: encode graph.json: %v", err)
	}
	f.Close()
	log.Printf("playground-capture: wrote %s (%d nodes, %d edges)", graphPath, len(dto.Nodes), len(dto.Edges))

	captured, skippedYaml := 0, 0
	for _, n := range dto.Nodes {
		if !n.Resolved || n.Kind == "Secret" {
			skippedYaml++
			continue // unresolved nodes have no manifest; Secret contents are never captured, same rule as the real /api/resource handler
		}
		gvr, ok := kindGVR[n.Kind]
		if !ok {
			log.Printf("playground-capture: no GVR mapping for kind %q (%s), skipping manifest", n.Kind, n.ID)
			skippedYaml++
			continue
		}
		if err := captureManifest(ctx, client, gvr, n, *out); err != nil {
			log.Printf("playground-capture: fetch manifest for %s: %v", n.ID, err)
			skippedYaml++
			continue
		}
		captured++
	}
	log.Printf("playground-capture: wrote %d manifests (%d skipped — Secrets/unresolved/unmapped)", captured, skippedYaml)
}

// captureManifest fetches one resource's manifest through the dynamic client,
// strips managedFields the same way the real handler does, and writes it under
// resources/<sanitized-id>.yaml.
func captureManifest(ctx context.Context, client *mantiskube.Client, gvr schema.GroupVersionResource, n engine.NodeDTO, out string) error {
	var obj *unstructured.Unstructured
	var err error
	if n.Namespace == "" {
		obj, err = client.Dynamic.Resource(gvr).Get(ctx, n.Name, metav1.GetOptions{})
	} else {
		obj, err = client.Dynamic.Resource(gvr).Namespace(n.Namespace).Get(ctx, n.Name, metav1.GetOptions{})
	}
	if err != nil {
		return err
	}

	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	data, err := yaml.Marshal(obj.Object)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(out, "resources", sanitizeID(n.ID)+".yaml"), data, 0o644)
}

// pruneForeignClusterScoped drops cluster-scoped nodes (Nodes, PersistentVolumes)
// that this scenario's namespace has no actual relationship to.
//
// BuildNamespaceGraph lists every physical Node and PersistentVolume in the
// whole cluster unconditionally, so runs-on/bound-to edges can resolve — that's
// correct for the real product, where a cluster only has one such set anyway.
// This capture tool instead runs the same namespace build repeatedly against
// one shared dev cluster carrying multiple scenarios' resources side by side, so
// without this step a scenario's fixture would leak other scenarios' PVs (e.g.
// a stray PersistentVolume from a namespace it never touches) as disconnected
// noise. A cluster-scoped node earns its place in the fixture only by actually
// appearing on one of this namespace's edges.
func pruneForeignClusterScoped(dto *engine.GraphDTO, namespace string) {
	touched := map[string]bool{}
	for _, e := range dto.Edges {
		touched[e.From] = true
		touched[e.To] = true
	}

	kept := make([]engine.NodeDTO, 0, len(dto.Nodes))
	keptIDs := map[string]bool{}
	for _, n := range dto.Nodes {
		if n.Namespace == namespace || (n.Namespace == "" && touched[n.ID]) {
			kept = append(kept, n)
			keptIDs[n.ID] = true
		}
	}
	dto.Nodes = kept

	edges := make([]engine.EdgeDTO, 0, len(dto.Edges))
	for _, e := range dto.Edges {
		if keptIDs[e.From] && keptIDs[e.To] {
			edges = append(edges, e)
		}
	}
	dto.Edges = edges

	dto.Meta.NodeCount = len(dto.Nodes)
	dto.Meta.EdgeCount = len(dto.Edges)
}

// sanitizeID turns a node's "<namespace>/<Kind>/<Name>" DTO id into a safe
// filename — the same id the frontend already has on every node, so it can
// derive the fixture path with no extra lookup table.
func sanitizeID(id string) string {
	return strings.ReplaceAll(id, "/", "__")
}
