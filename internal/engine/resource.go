package engine

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// getTimeout bounds a single resource fetch.
const getTimeout = 10 * time.Second

// kindGVR maps the kinds Mantis surfaces in the graph to the GroupVersionResource
// the dynamic client fetches them by. It is a static table rather than a
// discovery-backed RESTMapper because Mantis understands a fixed, known set of
// kinds — the same ones the graph builder lists — so a lookup table is simpler
// and needs no extra API round-trips.
var kindGVR = map[string]schema.GroupVersionResource{
	"Pod":                     {Group: "", Version: "v1", Resource: "pods"},
	"Service":                 {Group: "", Version: "v1", Resource: "services"},
	"ConfigMap":               {Group: "", Version: "v1", Resource: "configmaps"},
	"PersistentVolumeClaim":   {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
	"PersistentVolume":        {Group: "", Version: "v1", Resource: "persistentvolumes"},
	"Node":                    {Group: "", Version: "v1", Resource: "nodes"},
	"ResourceQuota":           {Group: "", Version: "v1", Resource: "resourcequotas"},
	"LimitRange":              {Group: "", Version: "v1", Resource: "limitranges"},
	"Deployment":              {Group: "apps", Version: "v1", Resource: "deployments"},
	"ReplicaSet":              {Group: "apps", Version: "v1", Resource: "replicasets"},
	"StatefulSet":             {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":               {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"Job":                     {Group: "batch", Version: "v1", Resource: "jobs"},
	"CronJob":                 {Group: "batch", Version: "v1", Resource: "cronjobs"},
	"Ingress":                 {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	"HorizontalPodAutoscaler": {Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
	"VerticalPodAutoscaler":   {Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers"},
	"NodePool":                {Group: "karpenter.sh", Version: "v1", Resource: "nodepools"},
}

// handleResource serves the raw YAML manifest of a single resource, addressed by
// ?kind=&name=&ns= query parameters. It reads through the dynamic client so it
// covers CRDs too, and strips server-managed noise (managedFields) for
// readability.
//
// Secrets are deliberately never served: the check below happens before
// anything is fetched, so a Secret's contents never leave the Kubernetes API
// server, let alone reach this handler or the browser — this is not a case of
// reading the value and then declining to forward it. The endpoint returns
// 403 with the two-line explanation the UI shows in place of the manifest.
func (s *Server) handleResource(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	namespace := r.URL.Query().Get("ns")

	if kind == "" || name == "" {
		http.Error(w, "kind and name are required", http.StatusBadRequest)
		return
	}

	if kind == "Secret" {
		http.Error(w, "Secret contents are hidden for security.\nMantis never displays Secret data through the UI.", http.StatusForbidden)
		return
	}

	gvr, ok := kindGVR[kind]
	if !ok {
		http.Error(w, "unsupported kind: "+kind, http.StatusBadRequest)
		return
	}

	if s.client.Dynamic == nil {
		http.Error(w, "dynamic client unavailable", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), getTimeout)
	defer cancel()

	obj, err := getResource(ctx, s, gvr, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, "resource not found", http.StatusNotFound)
			return
		}
		if apierrors.IsForbidden(err) {
			http.Error(w, "Read forbidden by RBAC. Access follows your Kubernetes permissions.", http.StatusForbidden)
			return
		}
		slog.Error("resource_fetch", "event", "kubernetes_api_failure", "kind", kind, "namespace", namespace, "name", name, "error", err.Error())
		http.Error(w, "fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Drop server-managed noise so the manifest reads like something a human wrote.
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	out, err := yaml.Marshal(obj.Object)
	if err != nil {
		http.Error(w, "encode yaml: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(out)
}

// getResource fetches one resource by GVR, choosing the namespaced or
// cluster-scoped call based on whether a namespace was given.
func getResource(ctx context.Context, s *Server, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if namespace == "" {
		return s.client.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	return s.client.Dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}
