// Code generated from a table in this change; one small adapter per kind the
// graph builder lists. Each embeds the real typed client (so every method
// keeps working) and overrides List — and, for ConfigMaps, Get — to read the
// watch cache. See watchcache.go.

package engine

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	typedautoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2"
	typedbatchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	typeddiscoveryv1 "k8s.io/client-go/kubernetes/typed/discovery/v1"
	typednetworkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	typedrbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	typedstoragev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
)

type corev1Client = typedcorev1.CoreV1Interface
type cCore struct {
	typedcorev1.CoreV1Interface
	wc *watchCache
}

func (x cCore) Namespaces() typedcorev1.NamespaceInterface {
	return cNamespace{x.CoreV1Interface.Namespaces(), x.wc, ""}
}

type cNamespace struct {
	typedcorev1.NamespaceInterface
	wc *watchCache
	ns string
}

func (x cNamespace) List(_ context.Context, _ metav1.ListOptions) (*corev1.NamespaceList, error) {
	return listOf(x.wc, "namespaces", x.ns, func(i []corev1.Namespace) *corev1.NamespaceList { return &corev1.NamespaceList{Items: i} })
}

func (x cCore) Pods(ns string) typedcorev1.PodInterface {
	return cPod{x.CoreV1Interface.Pods(ns), x.wc, ns}
}

type cPod struct {
	typedcorev1.PodInterface
	wc *watchCache
	ns string
}

func (x cPod) List(_ context.Context, _ metav1.ListOptions) (*corev1.PodList, error) {
	return listOf(x.wc, "pods", x.ns, func(i []corev1.Pod) *corev1.PodList { return &corev1.PodList{Items: i} })
}

func (x cCore) Services(ns string) typedcorev1.ServiceInterface {
	return cService{x.CoreV1Interface.Services(ns), x.wc, ns}
}

type cService struct {
	typedcorev1.ServiceInterface
	wc *watchCache
	ns string
}

func (x cService) List(_ context.Context, _ metav1.ListOptions) (*corev1.ServiceList, error) {
	return listOf(x.wc, "services", x.ns, func(i []corev1.Service) *corev1.ServiceList { return &corev1.ServiceList{Items: i} })
}

func (x cCore) ConfigMaps(ns string) typedcorev1.ConfigMapInterface {
	return cConfigMap{x.CoreV1Interface.ConfigMaps(ns), x.wc, ns}
}

type cConfigMap struct {
	typedcorev1.ConfigMapInterface
	wc *watchCache
	ns string
}

func (x cConfigMap) List(_ context.Context, _ metav1.ListOptions) (*corev1.ConfigMapList, error) {
	return listOf(x.wc, "configmaps", x.ns, func(i []corev1.ConfigMap) *corev1.ConfigMapList { return &corev1.ConfigMapList{Items: i} })
}

func (x cCore) Secrets(ns string) typedcorev1.SecretInterface {
	return cSecret{x.CoreV1Interface.Secrets(ns), x.wc, ns}
}

type cSecret struct {
	typedcorev1.SecretInterface
	wc *watchCache
	ns string
}

func (x cSecret) List(_ context.Context, _ metav1.ListOptions) (*corev1.SecretList, error) {
	return listOf(x.wc, "secrets", x.ns, func(i []corev1.Secret) *corev1.SecretList { return &corev1.SecretList{Items: i} })
}

func (x cCore) PersistentVolumeClaims(ns string) typedcorev1.PersistentVolumeClaimInterface {
	return cPersistentVolumeClaim{x.CoreV1Interface.PersistentVolumeClaims(ns), x.wc, ns}
}

type cPersistentVolumeClaim struct {
	typedcorev1.PersistentVolumeClaimInterface
	wc *watchCache
	ns string
}

func (x cPersistentVolumeClaim) List(_ context.Context, _ metav1.ListOptions) (*corev1.PersistentVolumeClaimList, error) {
	return listOf(x.wc, "persistentvolumeclaims", x.ns, func(i []corev1.PersistentVolumeClaim) *corev1.PersistentVolumeClaimList {
		return &corev1.PersistentVolumeClaimList{Items: i}
	})
}

func (x cCore) PersistentVolumes() typedcorev1.PersistentVolumeInterface {
	return cPersistentVolume{x.CoreV1Interface.PersistentVolumes(), x.wc, ""}
}

type cPersistentVolume struct {
	typedcorev1.PersistentVolumeInterface
	wc *watchCache
	ns string
}

func (x cPersistentVolume) List(_ context.Context, _ metav1.ListOptions) (*corev1.PersistentVolumeList, error) {
	return listOf(x.wc, "persistentvolumes", x.ns, func(i []corev1.PersistentVolume) *corev1.PersistentVolumeList {
		return &corev1.PersistentVolumeList{Items: i}
	})
}

func (x cCore) ResourceQuotas(ns string) typedcorev1.ResourceQuotaInterface {
	return cResourceQuota{x.CoreV1Interface.ResourceQuotas(ns), x.wc, ns}
}

type cResourceQuota struct {
	typedcorev1.ResourceQuotaInterface
	wc *watchCache
	ns string
}

func (x cResourceQuota) List(_ context.Context, _ metav1.ListOptions) (*corev1.ResourceQuotaList, error) {
	return listOf(x.wc, "resourcequotas", x.ns, func(i []corev1.ResourceQuota) *corev1.ResourceQuotaList { return &corev1.ResourceQuotaList{Items: i} })
}

func (x cCore) LimitRanges(ns string) typedcorev1.LimitRangeInterface {
	return cLimitRange{x.CoreV1Interface.LimitRanges(ns), x.wc, ns}
}

type cLimitRange struct {
	typedcorev1.LimitRangeInterface
	wc *watchCache
	ns string
}

func (x cLimitRange) List(_ context.Context, _ metav1.ListOptions) (*corev1.LimitRangeList, error) {
	return listOf(x.wc, "limitranges", x.ns, func(i []corev1.LimitRange) *corev1.LimitRangeList { return &corev1.LimitRangeList{Items: i} })
}

func (x cCore) Nodes() typedcorev1.NodeInterface {
	return cNode{x.CoreV1Interface.Nodes(), x.wc, ""}
}

type cNode struct {
	typedcorev1.NodeInterface
	wc *watchCache
	ns string
}

func (x cNode) List(_ context.Context, _ metav1.ListOptions) (*corev1.NodeList, error) {
	return listOf(x.wc, "nodes", x.ns, func(i []corev1.Node) *corev1.NodeList { return &corev1.NodeList{Items: i} })
}

func (x cCore) ServiceAccounts(ns string) typedcorev1.ServiceAccountInterface {
	return cServiceAccount{x.CoreV1Interface.ServiceAccounts(ns), x.wc, ns}
}

type cServiceAccount struct {
	typedcorev1.ServiceAccountInterface
	wc *watchCache
	ns string
}

func (x cServiceAccount) List(_ context.Context, _ metav1.ListOptions) (*corev1.ServiceAccountList, error) {
	return listOf(x.wc, "serviceaccounts", x.ns, func(i []corev1.ServiceAccount) *corev1.ServiceAccountList {
		return &corev1.ServiceAccountList{Items: i}
	})
}

type appsv1Client = typedappsv1.AppsV1Interface
type cApps struct {
	typedappsv1.AppsV1Interface
	wc *watchCache
}

func (x cApps) Deployments(ns string) typedappsv1.DeploymentInterface {
	return cDeployment{x.AppsV1Interface.Deployments(ns), x.wc, ns}
}

type cDeployment struct {
	typedappsv1.DeploymentInterface
	wc *watchCache
	ns string
}

func (x cDeployment) List(_ context.Context, _ metav1.ListOptions) (*appsv1.DeploymentList, error) {
	return listOf(x.wc, "deployments", x.ns, func(i []appsv1.Deployment) *appsv1.DeploymentList { return &appsv1.DeploymentList{Items: i} })
}

func (x cApps) ReplicaSets(ns string) typedappsv1.ReplicaSetInterface {
	return cReplicaSet{x.AppsV1Interface.ReplicaSets(ns), x.wc, ns}
}

type cReplicaSet struct {
	typedappsv1.ReplicaSetInterface
	wc *watchCache
	ns string
}

func (x cReplicaSet) List(_ context.Context, _ metav1.ListOptions) (*appsv1.ReplicaSetList, error) {
	return listOf(x.wc, "replicasets", x.ns, func(i []appsv1.ReplicaSet) *appsv1.ReplicaSetList { return &appsv1.ReplicaSetList{Items: i} })
}

func (x cApps) StatefulSets(ns string) typedappsv1.StatefulSetInterface {
	return cStatefulSet{x.AppsV1Interface.StatefulSets(ns), x.wc, ns}
}

type cStatefulSet struct {
	typedappsv1.StatefulSetInterface
	wc *watchCache
	ns string
}

func (x cStatefulSet) List(_ context.Context, _ metav1.ListOptions) (*appsv1.StatefulSetList, error) {
	return listOf(x.wc, "statefulsets", x.ns, func(i []appsv1.StatefulSet) *appsv1.StatefulSetList { return &appsv1.StatefulSetList{Items: i} })
}

func (x cApps) DaemonSets(ns string) typedappsv1.DaemonSetInterface {
	return cDaemonSet{x.AppsV1Interface.DaemonSets(ns), x.wc, ns}
}

type cDaemonSet struct {
	typedappsv1.DaemonSetInterface
	wc *watchCache
	ns string
}

func (x cDaemonSet) List(_ context.Context, _ metav1.ListOptions) (*appsv1.DaemonSetList, error) {
	return listOf(x.wc, "daemonsets", x.ns, func(i []appsv1.DaemonSet) *appsv1.DaemonSetList { return &appsv1.DaemonSetList{Items: i} })
}

type batchv1Client = typedbatchv1.BatchV1Interface
type cBatch struct {
	typedbatchv1.BatchV1Interface
	wc *watchCache
}

func (x cBatch) Jobs(ns string) typedbatchv1.JobInterface {
	return cJob{x.BatchV1Interface.Jobs(ns), x.wc, ns}
}

type cJob struct {
	typedbatchv1.JobInterface
	wc *watchCache
	ns string
}

func (x cJob) List(_ context.Context, _ metav1.ListOptions) (*batchv1.JobList, error) {
	return listOf(x.wc, "jobs", x.ns, func(i []batchv1.Job) *batchv1.JobList { return &batchv1.JobList{Items: i} })
}

func (x cBatch) CronJobs(ns string) typedbatchv1.CronJobInterface {
	return cCronJob{x.BatchV1Interface.CronJobs(ns), x.wc, ns}
}

type cCronJob struct {
	typedbatchv1.CronJobInterface
	wc *watchCache
	ns string
}

func (x cCronJob) List(_ context.Context, _ metav1.ListOptions) (*batchv1.CronJobList, error) {
	return listOf(x.wc, "cronjobs", x.ns, func(i []batchv1.CronJob) *batchv1.CronJobList { return &batchv1.CronJobList{Items: i} })
}

type discClient = typeddiscoveryv1.DiscoveryV1Interface
type cDisc struct {
	typeddiscoveryv1.DiscoveryV1Interface
	wc *watchCache
}

func (x cDisc) EndpointSlices(ns string) typeddiscoveryv1.EndpointSliceInterface {
	return cEndpointSlice{x.DiscoveryV1Interface.EndpointSlices(ns), x.wc, ns}
}

type cEndpointSlice struct {
	typeddiscoveryv1.EndpointSliceInterface
	wc *watchCache
	ns string
}

func (x cEndpointSlice) List(_ context.Context, _ metav1.ListOptions) (*discoveryv1.EndpointSliceList, error) {
	return listOf(x.wc, "endpointslices", x.ns, func(i []discoveryv1.EndpointSlice) *discoveryv1.EndpointSliceList {
		return &discoveryv1.EndpointSliceList{Items: i}
	})
}

type netClient = typednetworkingv1.NetworkingV1Interface
type cNet struct {
	typednetworkingv1.NetworkingV1Interface
	wc *watchCache
}

func (x cNet) Ingresses(ns string) typednetworkingv1.IngressInterface {
	return cIngress{x.NetworkingV1Interface.Ingresses(ns), x.wc, ns}
}

type cIngress struct {
	typednetworkingv1.IngressInterface
	wc *watchCache
	ns string
}

func (x cIngress) List(_ context.Context, _ metav1.ListOptions) (*networkingv1.IngressList, error) {
	return listOf(x.wc, "ingresses", x.ns, func(i []networkingv1.Ingress) *networkingv1.IngressList { return &networkingv1.IngressList{Items: i} })
}

func (x cNet) NetworkPolicies(ns string) typednetworkingv1.NetworkPolicyInterface {
	return cNetworkPolicy{x.NetworkingV1Interface.NetworkPolicies(ns), x.wc, ns}
}

type cNetworkPolicy struct {
	typednetworkingv1.NetworkPolicyInterface
	wc *watchCache
	ns string
}

func (x cNetworkPolicy) List(_ context.Context, _ metav1.ListOptions) (*networkingv1.NetworkPolicyList, error) {
	return listOf(x.wc, "networkpolicies", x.ns, func(i []networkingv1.NetworkPolicy) *networkingv1.NetworkPolicyList {
		return &networkingv1.NetworkPolicyList{Items: i}
	})
}

type rbacClient = typedrbacv1.RbacV1Interface
type cRbac struct {
	typedrbacv1.RbacV1Interface
	wc *watchCache
}

func (x cRbac) Roles(ns string) typedrbacv1.RoleInterface {
	return cRole{x.RbacV1Interface.Roles(ns), x.wc, ns}
}

type cRole struct {
	typedrbacv1.RoleInterface
	wc *watchCache
	ns string
}

func (x cRole) List(_ context.Context, _ metav1.ListOptions) (*rbacv1.RoleList, error) {
	return listOf(x.wc, "roles", x.ns, func(i []rbacv1.Role) *rbacv1.RoleList { return &rbacv1.RoleList{Items: i} })
}

func (x cRbac) RoleBindings(ns string) typedrbacv1.RoleBindingInterface {
	return cRoleBinding{x.RbacV1Interface.RoleBindings(ns), x.wc, ns}
}

type cRoleBinding struct {
	typedrbacv1.RoleBindingInterface
	wc *watchCache
	ns string
}

func (x cRoleBinding) List(_ context.Context, _ metav1.ListOptions) (*rbacv1.RoleBindingList, error) {
	return listOf(x.wc, "rolebindings", x.ns, func(i []rbacv1.RoleBinding) *rbacv1.RoleBindingList { return &rbacv1.RoleBindingList{Items: i} })
}

func (x cRbac) ClusterRoles() typedrbacv1.ClusterRoleInterface {
	return cClusterRole{x.RbacV1Interface.ClusterRoles(), x.wc, ""}
}

type cClusterRole struct {
	typedrbacv1.ClusterRoleInterface
	wc *watchCache
	ns string
}

func (x cClusterRole) List(_ context.Context, _ metav1.ListOptions) (*rbacv1.ClusterRoleList, error) {
	return listOf(x.wc, "clusterroles", x.ns, func(i []rbacv1.ClusterRole) *rbacv1.ClusterRoleList { return &rbacv1.ClusterRoleList{Items: i} })
}

func (x cRbac) ClusterRoleBindings() typedrbacv1.ClusterRoleBindingInterface {
	return cClusterRoleBinding{x.RbacV1Interface.ClusterRoleBindings(), x.wc, ""}
}

type cClusterRoleBinding struct {
	typedrbacv1.ClusterRoleBindingInterface
	wc *watchCache
	ns string
}

func (x cClusterRoleBinding) List(_ context.Context, _ metav1.ListOptions) (*rbacv1.ClusterRoleBindingList, error) {
	return listOf(x.wc, "clusterrolebindings", x.ns, func(i []rbacv1.ClusterRoleBinding) *rbacv1.ClusterRoleBindingList {
		return &rbacv1.ClusterRoleBindingList{Items: i}
	})
}

type storageClient = typedstoragev1.StorageV1Interface
type cStorage struct {
	typedstoragev1.StorageV1Interface
	wc *watchCache
}

func (x cStorage) StorageClasses() typedstoragev1.StorageClassInterface {
	return cStorageClass{x.StorageV1Interface.StorageClasses(), x.wc, ""}
}

type cStorageClass struct {
	typedstoragev1.StorageClassInterface
	wc *watchCache
	ns string
}

func (x cStorageClass) List(_ context.Context, _ metav1.ListOptions) (*storagev1.StorageClassList, error) {
	return listOf(x.wc, "storageclasses", x.ns, func(i []storagev1.StorageClass) *storagev1.StorageClassList {
		return &storagev1.StorageClassList{Items: i}
	})
}

type hpaClient = typedautoscalingv2.AutoscalingV2Interface
type cHPA struct {
	typedautoscalingv2.AutoscalingV2Interface
	wc *watchCache
}

func (x cHPA) HorizontalPodAutoscalers(ns string) typedautoscalingv2.HorizontalPodAutoscalerInterface {
	return cHorizontalPodAutoscaler{x.AutoscalingV2Interface.HorizontalPodAutoscalers(ns), x.wc, ns}
}

type cHorizontalPodAutoscaler struct {
	typedautoscalingv2.HorizontalPodAutoscalerInterface
	wc *watchCache
	ns string
}

func (x cHorizontalPodAutoscaler) List(_ context.Context, _ metav1.ListOptions) (*autoscalingv2.HorizontalPodAutoscalerList, error) {
	return listOf(x.wc, "horizontalpodautoscalers", x.ns, func(i []autoscalingv2.HorizontalPodAutoscaler) *autoscalingv2.HorizontalPodAutoscalerList {
		return &autoscalingv2.HorizontalPodAutoscalerList{Items: i}
	})
}

// Get on ConfigMaps is the one non-List read the builder makes (detecting the
// cluster-autoscaler's status ConfigMap).
func (x cConfigMap) Get(_ context.Context, name string, _ metav1.GetOptions) (*corev1.ConfigMap, error) {
	it, err := items[corev1.ConfigMap](x.wc, "configmaps", x.ns)
	if err != nil {
		return nil, err
	}
	for i := range it {
		if it[i].Name == name {
			return &it[i], nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, name)
}

var (
	_ = appsv1.Deployment{}
	_ = autoscalingv2.HorizontalPodAutoscaler{}
	_ = batchv1.Job{}
	_ = discoveryv1.EndpointSlice{}
	_ = networkingv1.Ingress{}
	_ = rbacv1.Role{}
	_ = storagev1.StorageClass{}
)
