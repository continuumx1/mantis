package graph

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// podHealthNote is a compact one-line health summary for the map, e.g.
// "Running · 1/1", "Pending · 0/1", or "CrashLoopBackOff · 0/1 · restarts 5".
// It is intentionally terser than render.containerHealth, which is the detailed
// per-container form used by `inspect pod`.
func podHealthNote(pod *corev1.Pod) string {
	total := len(pod.Spec.Containers)
	ready := 0
	var restarts int32
	headline := string(pod.Status.Phase)

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += cs.RestartCount
		if cs.State.Waiting != nil && isProblemReason(cs.State.Waiting.Reason) {
			headline = cs.State.Waiting.Reason
		}
	}

	note := fmt.Sprintf("%s · %d/%d", headline, ready, total)
	if restarts > 0 {
		note += fmt.Sprintf(" · restarts %d", restarts)
	}
	return note
}

// isProblemReason reports whether a container waiting reason is a real fault
// worth surfacing as the headline (as opposed to a normal transient state such
// as ContainerCreating or PodInitializing).
func isProblemReason(reason string) bool {
	switch reason {
	case "CrashLoopBackOff",
		"ImagePullBackOff",
		"ErrImagePull",
		"CreateContainerConfigError",
		"CreateContainerError",
		"InvalidImageName",
		"RunContainerError":
		return true
	default:
		return false
	}
}

// pvcAttributes summarizes a PersistentVolumeClaim as phase, size, access modes,
// and storage class, e.g. ["Bound", "10Gi", "RWO", "sc:standard"].
func pvcAttributes(pvc *corev1.PersistentVolumeClaim) []string {
	var attrs []string
	if pvc.Status.Phase != "" {
		attrs = append(attrs, string(pvc.Status.Phase))
	}
	if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		attrs = append(attrs, q.String())
	} else if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		attrs = append(attrs, q.String())
	}
	if modes := accessModesShort(pvc.Spec.AccessModes); modes != "" {
		attrs = append(attrs, modes)
	}
	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
		attrs = append(attrs, "sc:"+*pvc.Spec.StorageClassName)
	}
	return attrs
}

// pvAttributes summarizes a PersistentVolume as size, reclaim policy, and phase,
// e.g. ["10Gi", "Retain", "Bound"].
func pvAttributes(pv *corev1.PersistentVolume) []string {
	var attrs []string
	if q, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
		attrs = append(attrs, q.String())
	}
	if pv.Spec.PersistentVolumeReclaimPolicy != "" {
		attrs = append(attrs, string(pv.Spec.PersistentVolumeReclaimPolicy))
	}
	if pv.Status.Phase != "" {
		attrs = append(attrs, string(pv.Status.Phase))
	}
	return attrs
}

// accessModesShort renders access modes as short codes (RWO, ROX, RWX, RWOP).
func accessModesShort(modes []corev1.PersistentVolumeAccessMode) string {
	var codes []string
	for _, m := range modes {
		switch m {
		case corev1.ReadWriteOnce:
			codes = append(codes, "RWO")
		case corev1.ReadOnlyMany:
			codes = append(codes, "ROX")
		case corev1.ReadWriteMany:
			codes = append(codes, "RWX")
		case corev1.ReadWriteOncePod:
			codes = append(codes, "RWOP")
		}
	}
	return strings.Join(codes, ",")
}

// isSystemConfigMap reports whether a ConfigMap is cluster-managed noise that
// the map hides by default.
func isSystemConfigMap(name string) bool {
	return name == "kube-root-ca.crt"
}

// IsHiddenNamespace reports whether an entire namespace is cluster-managed
// plumbing that never belongs in front of a user, in any of the ways a
// namespace shows up: the topology, search, the namespace/region list, or
// the resource/namespace counters. Unlike isSystemConfigMap/isSystemSecret
// above (individual resources hidden but still present in the graph, so
// references to them keep resolving), a hidden namespace is skipped
// entirely at the source — see graph.BuildClusterGraphProgressive — so it
// costs no API calls and leaves no trace, not even an empty region.
//
// kube-node-lease holds nothing but per-Node Lease objects (the heartbeat
// mechanism node controllers use since KEP-0009) — a purely internal
// implementation detail Mantis has no model for and a user never has reason
// to look at. It is a standard namespace name defined by Kubernetes itself,
// not a distro convention, so this one check is already correct on every
// distribution (kubeadm, EKS, GKE, AKS, RKE2, minikube, kind, ...) without
// needing per-distro special-casing.
func IsHiddenNamespace(name string) bool {
	return name == "kube-node-lease"
}

// isSystemSecret reports whether a Secret is cluster-managed noise (service
// account tokens, Helm release state) that the map hides by default.
func isSystemSecret(secret *corev1.Secret) bool {
	switch secret.Type {
	case corev1.SecretTypeServiceAccountToken, "helm.sh/release.v1":
		return true
	default:
		return false
	}
}
