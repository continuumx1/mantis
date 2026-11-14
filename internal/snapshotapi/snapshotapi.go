// Package snapshotapi is the internal contract between mantis-engine and
// mantis-report: one consistent copy of the cluster as the engine has
// observed it — its published graph plus the objects that graph was built
// from. The engine serves it from its watch cache (no extra Kubernetes API
// calls); mantis-report consumes it and never talks to Kubernetes itself.
//
// Objects are trimmed before they leave the engine: Secret and ConfigMap
// values are never included, pod templates and container environment are
// dropped, and managedFields and last-applied-configuration are removed. A
// report only needs identity, status and counts.
package snapshotapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Path is where mantis-engine serves the snapshot. It is deliberately not
// under /api/: mantis-web only proxies /api/, so browsers cannot reach it.
const Path = "/internal/v1/snapshot"

// Response is the snapshot payload.
type Response struct {
	Revision    uint64          `json:"revision"`
	TakenAt     time.Time       `json:"takenAt"`
	SyncMode    string          `json:"syncMode"`
	Graph       json.RawMessage `json:"graph"` // the engine's GraphDTO, as /api/graph serves it
	Objects     Objects         `json:"objects"`
	NotReadable []string        `json:"notReadable,omitempty"`
}

// Objects are the trimmed Kubernetes objects behind the graph.
type Objects struct {
	Namespaces          []corev1.Namespace                      `json:"namespaces"`
	Pods                []corev1.Pod                            `json:"pods"`
	Deployments         []appsv1.Deployment                     `json:"deployments"`
	ReplicaSets         []appsv1.ReplicaSet                     `json:"replicaSets"`
	StatefulSets        []appsv1.StatefulSet                    `json:"statefulSets"`
	DaemonSets          []appsv1.DaemonSet                      `json:"daemonSets"`
	Jobs                []batchv1.Job                           `json:"jobs"`
	CronJobs            []batchv1.CronJob                       `json:"cronJobs"`
	Services            []corev1.Service                        `json:"services"`
	Ingresses           []networkingv1.Ingress                  `json:"ingresses"`
	NetworkPolicies     []networkingv1.NetworkPolicy            `json:"networkPolicies"`
	ConfigMaps          []corev1.ConfigMap                      `json:"configMaps"`
	Secrets             []corev1.Secret                         `json:"secrets"`
	PVCs                []corev1.PersistentVolumeClaim          `json:"persistentVolumeClaims"`
	PVs                 []corev1.PersistentVolume               `json:"persistentVolumes"`
	StorageClasses      []storagev1.StorageClass                `json:"storageClasses"`
	Nodes               []corev1.Node                           `json:"nodes"`
	ServiceAccounts     []corev1.ServiceAccount                 `json:"serviceAccounts"`
	Roles               []rbacv1.Role                           `json:"roles"`
	RoleBindings        []rbacv1.RoleBinding                    `json:"roleBindings"`
	ClusterRoles        []rbacv1.ClusterRole                    `json:"clusterRoles"`
	ClusterRoleBindings []rbacv1.ClusterRoleBinding             `json:"clusterRoleBindings"`
	HPAs                []autoscalingv2.HorizontalPodAutoscaler `json:"horizontalPodAutoscalers"`
	ResourceQuotas      []corev1.ResourceQuota                  `json:"resourceQuotas"`
	LimitRanges         []corev1.LimitRange                     `json:"limitRanges"`
}

// Collect reads every object kind through cs — in watch mode the engine's
// cache-backed client, so no API calls — and trims each one. A kind the
// identity may not list is recorded in notReadable; any other error fails,
// because a silently incomplete report is worse than none.
func Collect(ctx context.Context, cs kubernetes.Interface) (Objects, []string, error) {
	var o Objects
	var notReadable []string
	var firstErr error
	opts := metav1.ListOptions{}
	try := func(resource string, f func() error) {
		if firstErr != nil {
			return
		}
		if err := f(); err != nil {
			if apierrors.IsForbidden(err) {
				notReadable = append(notReadable, resource)
				return
			}
			firstErr = fmt.Errorf("list %s: %w", resource, err)
		}
	}
	core, apps, batch, net, rbac := cs.CoreV1(), cs.AppsV1(), cs.BatchV1(), cs.NetworkingV1(), cs.RbacV1()

	try("namespaces", func() error {
		l, err := core.Namespaces().List(ctx, opts)
		if err == nil {
			o.Namespaces = l.Items
		}
		return err
	})
	try("pods", func() error {
		l, err := core.Pods("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i] = trimPod(l.Items[i])
			}
			o.Pods = l.Items
		}
		return err
	})
	try("deployments", func() error {
		l, err := apps.Deployments("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Spec.Template = corev1.PodTemplateSpec{}
			}
			o.Deployments = l.Items
		}
		return err
	})
	try("replicasets", func() error {
		l, err := apps.ReplicaSets("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Spec.Template = corev1.PodTemplateSpec{}
			}
			o.ReplicaSets = l.Items
		}
		return err
	})
	try("statefulsets", func() error {
		l, err := apps.StatefulSets("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Spec.Template = corev1.PodTemplateSpec{}
				l.Items[i].Spec.VolumeClaimTemplates = nil
			}
			o.StatefulSets = l.Items
		}
		return err
	})
	try("daemonsets", func() error {
		l, err := apps.DaemonSets("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Spec.Template = corev1.PodTemplateSpec{}
			}
			o.DaemonSets = l.Items
		}
		return err
	})
	try("jobs", func() error {
		l, err := batch.Jobs("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Spec.Template = corev1.PodTemplateSpec{}
			}
			o.Jobs = l.Items
		}
		return err
	})
	try("cronjobs", func() error {
		l, err := batch.CronJobs("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Spec.JobTemplate = batchv1.JobTemplateSpec{}
			}
			o.CronJobs = l.Items
		}
		return err
	})
	try("services", func() error {
		l, err := core.Services("").List(ctx, opts)
		if err == nil {
			o.Services = l.Items
		}
		return err
	})
	try("ingresses", func() error {
		l, err := net.Ingresses("").List(ctx, opts)
		if err == nil {
			o.Ingresses = l.Items
		}
		return err
	})
	try("networkpolicies", func() error {
		l, err := net.NetworkPolicies("").List(ctx, opts)
		if err == nil {
			o.NetworkPolicies = l.Items
		}
		return err
	})
	try("configmaps", func() error {
		l, err := core.ConfigMaps("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				// Only the key count is reported; values never leave the engine.
				keys := map[string]string{}
				for k := range l.Items[i].Data {
					keys[k] = ""
				}
				for k := range l.Items[i].BinaryData {
					keys[k] = ""
				}
				l.Items[i].Data, l.Items[i].BinaryData = keys, nil
			}
			o.ConfigMaps = l.Items
		}
		return err
	})
	try("secrets", func() error {
		l, err := core.Secrets("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Data, l.Items[i].StringData = nil, nil
			}
			o.Secrets = l.Items
		}
		return err
	})
	try("persistentvolumeclaims", func() error {
		l, err := core.PersistentVolumeClaims("").List(ctx, opts)
		if err == nil {
			o.PVCs = l.Items
		}
		return err
	})
	try("persistentvolumes", func() error {
		l, err := core.PersistentVolumes().List(ctx, opts)
		if err == nil {
			o.PVs = l.Items
		}
		return err
	})
	try("storageclasses", func() error {
		l, err := cs.StorageV1().StorageClasses().List(ctx, opts)
		if err == nil {
			o.StorageClasses = l.Items
		}
		return err
	})
	try("nodes", func() error {
		l, err := core.Nodes().List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Status.Images = nil // large and not reported
			}
			o.Nodes = l.Items
		}
		return err
	})
	try("serviceaccounts", func() error {
		l, err := core.ServiceAccounts("").List(ctx, opts)
		if err == nil {
			for i := range l.Items {
				l.Items[i].Secrets = nil
			}
			o.ServiceAccounts = l.Items
		}
		return err
	})
	try("roles", func() error {
		l, err := rbac.Roles("").List(ctx, opts)
		if err == nil {
			o.Roles = l.Items
		}
		return err
	})
	try("rolebindings", func() error {
		l, err := rbac.RoleBindings("").List(ctx, opts)
		if err == nil {
			o.RoleBindings = l.Items
		}
		return err
	})
	try("clusterroles", func() error {
		l, err := rbac.ClusterRoles().List(ctx, opts)
		if err == nil {
			o.ClusterRoles = l.Items
		}
		return err
	})
	try("clusterrolebindings", func() error {
		l, err := rbac.ClusterRoleBindings().List(ctx, opts)
		if err == nil {
			o.ClusterRoleBindings = l.Items
		}
		return err
	})
	try("horizontalpodautoscalers", func() error {
		l, err := cs.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, opts)
		if err == nil {
			o.HPAs = l.Items
		}
		return err
	})
	try("resourcequotas", func() error {
		l, err := core.ResourceQuotas("").List(ctx, opts)
		if err == nil {
			o.ResourceQuotas = l.Items
		}
		return err
	})
	try("limitranges", func() error {
		l, err := core.LimitRanges("").List(ctx, opts)
		if err == nil {
			o.LimitRanges = l.Items
		}
		return err
	})
	if firstErr != nil {
		return Objects{}, nil, firstErr
	}
	stripMeta(&o)
	return o, notReadable, nil
}

// trimPod keeps what a report shows (phase, readiness, restarts, node, IP,
// owner) and drops the rest. It builds new slices rather than editing shared
// ones, so a cached object is never modified.
func trimPod(p corev1.Pod) corev1.Pod {
	containers := make([]corev1.Container, len(p.Spec.Containers))
	for i, c := range p.Spec.Containers {
		containers[i] = corev1.Container{Name: c.Name, Image: c.Image}
	}
	p.Spec = corev1.PodSpec{NodeName: p.Spec.NodeName, ServiceAccountName: p.Spec.ServiceAccountName, Containers: containers}
	return p
}

// stripMeta removes managedFields and last-applied-configuration everywhere
// (the latter can carry a Secret's data verbatim). Annotation maps are
// replaced, not edited, so cached objects are never modified.
func stripMeta(o *Objects) {
	fix := func(m *metav1.ObjectMeta) {
		m.ManagedFields = nil
		if _, ok := m.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
			a := make(map[string]string, len(m.Annotations))
			for k, v := range m.Annotations {
				if k != "kubectl.kubernetes.io/last-applied-configuration" {
					a[k] = v
				}
			}
			m.Annotations = a
		}
	}
	for i := range o.Namespaces {
		fix(&o.Namespaces[i].ObjectMeta)
	}
	for i := range o.Pods {
		fix(&o.Pods[i].ObjectMeta)
	}
	for i := range o.Deployments {
		fix(&o.Deployments[i].ObjectMeta)
	}
	for i := range o.ReplicaSets {
		fix(&o.ReplicaSets[i].ObjectMeta)
	}
	for i := range o.StatefulSets {
		fix(&o.StatefulSets[i].ObjectMeta)
	}
	for i := range o.DaemonSets {
		fix(&o.DaemonSets[i].ObjectMeta)
	}
	for i := range o.Jobs {
		fix(&o.Jobs[i].ObjectMeta)
	}
	for i := range o.CronJobs {
		fix(&o.CronJobs[i].ObjectMeta)
	}
	for i := range o.Services {
		fix(&o.Services[i].ObjectMeta)
	}
	for i := range o.Ingresses {
		fix(&o.Ingresses[i].ObjectMeta)
	}
	for i := range o.NetworkPolicies {
		fix(&o.NetworkPolicies[i].ObjectMeta)
	}
	for i := range o.ConfigMaps {
		fix(&o.ConfigMaps[i].ObjectMeta)
	}
	for i := range o.Secrets {
		fix(&o.Secrets[i].ObjectMeta)
	}
	for i := range o.PVCs {
		fix(&o.PVCs[i].ObjectMeta)
	}
	for i := range o.PVs {
		fix(&o.PVs[i].ObjectMeta)
	}
	for i := range o.StorageClasses {
		fix(&o.StorageClasses[i].ObjectMeta)
	}
	for i := range o.Nodes {
		fix(&o.Nodes[i].ObjectMeta)
	}
	for i := range o.ServiceAccounts {
		fix(&o.ServiceAccounts[i].ObjectMeta)
	}
	for i := range o.Roles {
		fix(&o.Roles[i].ObjectMeta)
	}
	for i := range o.RoleBindings {
		fix(&o.RoleBindings[i].ObjectMeta)
	}
	for i := range o.ClusterRoles {
		fix(&o.ClusterRoles[i].ObjectMeta)
	}
	for i := range o.ClusterRoleBindings {
		fix(&o.ClusterRoleBindings[i].ObjectMeta)
	}
	for i := range o.HPAs {
		fix(&o.HPAs[i].ObjectMeta)
	}
	for i := range o.ResourceQuotas {
		fix(&o.ResourceQuotas[i].ObjectMeta)
	}
	for i := range o.LimitRanges {
		fix(&o.LimitRanges[i].ObjectMeta)
	}
}
