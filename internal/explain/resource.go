package explain

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/graph"
	"github.com/continuumx1/knw/internal/render"
)

// clusterScoped kinds have no namespace.
var clusterScoped = map[string]bool{
	"Node":             true,
	"PersistentVolume": true,
	"Namespace":        true,
}

// kindAliases maps CLI kind names (and common short forms) to the canonical
// Kind strings the graph uses.
var kindAliases = map[string]string{
	"pod":                   "Pod",
	"service":               "Service",
	"svc":                   "Service",
	"ingress":               "Ingress",
	"ing":                   "Ingress",
	"deployment":            "Deployment",
	"deploy":                "Deployment",
	"replicaset":            "ReplicaSet",
	"rs":                    "ReplicaSet",
	"statefulset":           "StatefulSet",
	"sts":                   "StatefulSet",
	"daemonset":             "DaemonSet",
	"ds":                    "DaemonSet",
	"job":                   "Job",
	"cronjob":               "CronJob",
	"configmap":             "ConfigMap",
	"cm":                    "ConfigMap",
	"secret":                "Secret",
	"persistentvolumeclaim": "PersistentVolumeClaim",
	"pvc":                   "PersistentVolumeClaim",
	"persistentvolume":      "PersistentVolume",
	"pv":                    "PersistentVolume",
	"node":                  "Node",
}

// MapResource renders a subject-centric graph view rooted at one resource: it
// builds the namespace graph (reusing the shared engine), then follows the
// resource's relationships outward. Namespaces are contextual, not boundaries —
// the subject view will grow to cross them as cross-namespace edges are added.
func MapResource(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	kind string,
	name string,
) (string, error) {
	canonical, ok := kindAliases[strings.ToLower(kind)]
	if !ok {
		return "", fmt.Errorf("unsupported kind %q", kind)
	}

	ns := namespace
	if clusterScoped[canonical] {
		ns = ""
	}
	root := graph.ResourceRef{Kind: canonical, Name: name, Namespace: ns}

	investigation, _, err := graph.BuildNamespaceGraph(ctx, clientset, namespace, true)
	if err != nil {
		return "", fmt.Errorf("build graph for namespace %q: %w", namespace, err)
	}

	if _, checked := investigation.Existence(root); !checked {
		return "", fmt.Errorf("%s/%s not found in namespace %q", canonical, name, namespace)
	}

	return render.ResourceGraph(root, investigation), nil
}
