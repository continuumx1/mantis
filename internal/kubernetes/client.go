package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	Clientset *kubernetes.Clientset
	Context   string
	Namespace string
	Server    string
}

func NewClient() (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}

	kubeconfig := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	currentContext := config.CurrentContext

	contextConfig, ok := config.Contexts[currentContext]
	if !ok {
		return nil, fmt.Errorf("context %q not found", currentContext)
	}

	namespace := contextConfig.Namespace
	if namespace == "" {
		namespace = "default"
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build Kubernetes config: %w", err)
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
