package kubernetes

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// serviceAccountNamespaceFile is where a Pod's ServiceAccount namespace is
// projected inside the container.
const serviceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// clientQPS and clientBurst raise the client-go rate limits well above the
// conservative defaults (5 QPS / 10 burst). A full-cluster graph fires many List
// calls per namespace — and one more per namespace as coverage grows — so the
// defaults throttle the build into a timeout on clusters with several
// namespaces. Mantis is read-only, so a higher read rate against the API server
// is safe.
const (
	clientQPS   = 50
	clientBurst = 100
)

// tuneRateLimits lifts the read throughput limits on a rest config so a
// whole-cluster read does not get throttled into a deadline.
func tuneRateLimits(c *rest.Config) {
	c.QPS = clientQPS
	c.Burst = clientBurst
}

type Client struct {
	Clientset *kubernetes.Clientset
	// Dynamic reads arbitrary resources by GVR, including CRDs (VPA, Karpenter)
	// that the typed Clientset does not know about. It is nil only if it could not
	// be built; callers must treat CRD collection as best-effort.
	Dynamic   dynamic.Interface
	Context   string
	Namespace string
	Server    string
}

// NewClient builds a read-only Kubernetes client that works both inside and
// outside a cluster. When Mantis runs as a Pod it uses the mounted ServiceAccount
// (in-cluster config); everywhere else it falls back to the standard kubeconfig
// resolution rules. This lets the same binary run as a microservice in a target
// cluster and as a local dev tool without a code change.
func NewClient() (*Client, error) {
	if client, err := newInClusterClient(); err == nil {
		return client, nil
	}
	return newKubeconfigClient()
}

// newInClusterClient builds a client from the Pod's ServiceAccount. It fails
// (and the caller falls back to kubeconfig) when not running inside a cluster.
func newInClusterClient() (*Client, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	tuneRateLimits(restConfig)

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create in-cluster Kubernetes client: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create in-cluster dynamic client: %w", err)
	}

	namespace := "default"
	if data, err := os.ReadFile(serviceAccountNamespaceFile); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			namespace = ns
		}
	}

	return &Client{
		Clientset: clientset,
		Dynamic:   dyn,
		Context:   "in-cluster",
		Namespace: namespace,
		Server:    restConfig.Host,
	}, nil
}

// newKubeconfigClient builds a client from the standard kubeconfig: it honours
// $KUBECONFIG (including multi-path lists), falls back to ~/.kube/config, and
// uses the kubeconfig's current context and namespace.
func newKubeconfigClient() (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	tuneRateLimits(restConfig)

	namespace, _, err := clientConfig.Namespace()
	if err != nil {
		return nil, fmt.Errorf("resolve namespace: %w", err)
	}
	if namespace == "" {
		namespace = "default"
	}

	currentContext := ""
	if rawConfig, err := clientConfig.RawConfig(); err == nil {
		currentContext = rawConfig.CurrentContext
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &Client{
		Clientset: clientset,
		Dynamic:   dyn,
		Context:   currentContext,
		Namespace: namespace,
		Server:    restConfig.Host,
	}, nil
}
