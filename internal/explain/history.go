package explain

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/history"
	"github.com/continuumx1/knw/internal/render"
)

// DeploymentHistory reconstructs a Deployment's rollout history and renders it.
func DeploymentHistory(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	name string,
) (string, error) {
	revisions, err := history.DeploymentRevisions(ctx, clientset, namespace, name)
	if err != nil {
		return "", err
	}

	return render.DeploymentHistory(name, revisions), nil
}
