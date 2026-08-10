package explain

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func PodWhy(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	name string,
) (string, error) {

	pod, err := clientset.CoreV1().
		Pods(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod %q: %w", name, err)
	}

	output := ""

	output += fmt.Sprintf("POD/%s\n\n", pod.Name)
	output += "WHY\n\n"

	// Ownership
	if len(pod.OwnerReferences) == 0 {
		output += "Origin\n"
		output += "  └── Directly created\n\n"
		output += "Owner\n"
		output += "  └── None\n\n"
	} else {
		output += "Ownership\n"

		for _, owner := range pod.OwnerReferences {
			output += fmt.Sprintf(
				"  └── %s/%s\n",
				owner.Kind,
				owner.Name,
			)

			if owner.Kind == "ReplicaSet" {
				rs, err := clientset.AppsV1().
					ReplicaSets(namespace).
					Get(ctx, owner.Name, metav1.GetOptions{})

				if err == nil {
					for _, rsOwner := range rs.OwnerReferences {
						output += fmt.Sprintf(
							"       └── %s/%s\n",
							rsOwner.Kind,
							rsOwner.Name,
						)
					}
				}
			}
		}

		output += "\n"
	}

	// Node
	output += "Runs on\n"

	if pod.Spec.NodeName != "" {
		output += fmt.Sprintf(
			"  └── Node/%s\n\n",
			pod.Spec.NodeName,
		)
	} else {
		output += "  └── Not scheduled\n\n"
	}

	// Status
	output += "Status\n"
	output += fmt.Sprintf(
		"  └── %s\n",
		pod.Status.Phase,
	)

	return output, nil
}
