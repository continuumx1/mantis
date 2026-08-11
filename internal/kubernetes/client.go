package kubernetes

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	Clientset *kubernetes.Clientset
	Context   string
	Namespace string
	Server    string
}

// NewClient builds a read-only Kubernetes client using the standard kubeconfig
// resolution rules: it honours $KUBECONFIG (including multi-path lists) and
// falls back to ~/.kube/config, and it uses the kubeconfig's current context and
// namespace.
func NewClient() (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

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

	return &Client{
		Clientset: clientset,
		Context:   currentContext,
		Namespace: namespace,
		Server:    restConfig.Host,
	}, nil
}
