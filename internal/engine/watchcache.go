package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// watchCache keeps an in-memory copy of every kind the graph builder reads,
// kept current by the Kubernetes list/watch protocol (client-go informers):
// one paginated cluster-wide LIST per kind at startup, then a WATCH stream of
// changes. Rebuilding the graph reads only this cache, so steady-state load
// on the API server is the watch streams — not a re-list of the whole
// cluster every interval.
//
// Lost or expired watches (HTTP 410) are recovered by the informers
// themselves: they re-list and resume, with resourceVersion bookkeeping
// handled by client-go's reflector.
type watchCache struct {
	typed   map[string]cache.SharedIndexInformer // resource -> informer
	dyn     map[schema.GroupVersionResource]cache.SharedIndexInformer
	denied  map[string]error // resource -> why it is not watched (forbidden / absent)
	factory informers.SharedInformerFactory
	dynFac  dynamicinformer.DynamicSharedInformerFactory
	changed chan struct{} // signalled (non-blocking) on any add/update/delete
	// watchDenied is closed if the API server refuses a WATCH it allowed a
	// LIST for (RBAC granting list but not watch): watch mode cannot keep the
	// graph current then, and the engine falls back to polling.
	watchDenied chan struct{}
	denyOnce    sync.Once
	events      atomic.Int64 // change notifications received
	mu          sync.RWMutex
}

// errWatchDenied means RBAC allows listing but not watching.
var errWatchDenied = errors.New("watch is not permitted (list is): grant the \"watch\" verb, or use MANTIS_SYNC_MODE=poll")

// initialSyncTimeout bounds the first paginated list of every kind; a
// cluster that cannot be read in this time falls back to poll mode.
var initialSyncTimeout = 3 * time.Minute

// watchedKind describes one kind the graph builder lists.
type watchedKind struct {
	resource string // plural, as recorded in "skipped"
	gvr      schema.GroupVersionResource
	probe    func(ctx context.Context, cs kubernetes.Interface, o metav1.ListOptions) error
}

// lst probes a kind with a LIMIT=1 list. Watch permission is not probed
// separately: opening a probe watch would stream every existing object. A
// denied watch is caught by the informers' watch error handler instead (see
// watchDenied).
func lst[T any](f func(context.Context, metav1.ListOptions) (T, error)) func(context.Context, metav1.ListOptions) error {
	return func(ctx context.Context, o metav1.ListOptions) error { _, err := f(ctx, o); return err }
}

// watchedKinds is every kind graph.BuildClusterGraph lists, in one table.
func watchedKinds() []watchedKind {
	k := func(res, group, version string, probe func(context.Context, kubernetes.Interface, metav1.ListOptions) error) watchedKind {
		return watchedKind{resource: res, gvr: schema.GroupVersionResource{Group: group, Version: version, Resource: res}, probe: probe}
	}
	return []watchedKind{
		k("namespaces", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().Namespaces().List)(c, o)
		}),
		k("pods", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().Pods("").List)(c, o)
		}),
		k("services", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().Services("").List)(c, o)
		}),
		k("configmaps", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().ConfigMaps("").List)(c, o)
		}),
		k("secrets", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().Secrets("").List)(c, o)
		}),
		k("persistentvolumeclaims", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().PersistentVolumeClaims("").List)(c, o)
		}),
		k("persistentvolumes", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().PersistentVolumes().List)(c, o)
		}),
		k("resourcequotas", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().ResourceQuotas("").List)(c, o)
		}),
		k("limitranges", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().LimitRanges("").List)(c, o)
		}),
		k("nodes", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().Nodes().List)(c, o)
		}),
		k("serviceaccounts", "", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.CoreV1().ServiceAccounts("").List)(c, o)
		}),
		k("deployments", "apps", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.AppsV1().Deployments("").List)(c, o)
		}),
		k("replicasets", "apps", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.AppsV1().ReplicaSets("").List)(c, o)
		}),
		k("statefulsets", "apps", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.AppsV1().StatefulSets("").List)(c, o)
		}),
		k("daemonsets", "apps", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.AppsV1().DaemonSets("").List)(c, o)
		}),
		k("jobs", "batch", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.BatchV1().Jobs("").List)(c, o)
		}),
		k("cronjobs", "batch", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.BatchV1().CronJobs("").List)(c, o)
		}),
		k("endpointslices", "discovery.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.DiscoveryV1().EndpointSlices("").List)(c, o)
		}),
		k("ingresses", "networking.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.NetworkingV1().Ingresses("").List)(c, o)
		}),
		k("networkpolicies", "networking.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.NetworkingV1().NetworkPolicies("").List)(c, o)
		}),
		k("horizontalpodautoscalers", "autoscaling", "v2", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.AutoscalingV2().HorizontalPodAutoscalers("").List)(c, o)
		}),
		k("roles", "rbac.authorization.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.RbacV1().Roles("").List)(c, o)
		}),
		k("rolebindings", "rbac.authorization.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.RbacV1().RoleBindings("").List)(c, o)
		}),
		k("clusterroles", "rbac.authorization.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.RbacV1().ClusterRoles().List)(c, o)
		}),
		k("clusterrolebindings", "rbac.authorization.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.RbacV1().ClusterRoleBindings().List)(c, o)
		}),
		k("storageclasses", "storage.k8s.io", "v1", func(c context.Context, cs kubernetes.Interface, o metav1.ListOptions) error {
			return lst(cs.StorageV1().StorageClasses().List)(c, o)
		}),
	}
}

// Optional CRDs read through the dynamic client (absent on most clusters).
var watchedCRDs = []schema.GroupVersionResource{
	{Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers"},
	{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"},
	{Group: "karpenter.sh", Version: "v1beta1", Resource: "nodepools"},
}

// stripForCache runs on every object before it is stored. It keeps the cache
// small and — for Secrets — makes sure values never enter Mantis's memory at
// all, rather than being read and dropped later.
func stripForCache(obj any) (any, error) {
	if s, ok := obj.(*corev1.Secret); ok {
		s.Data, s.StringData = nil, nil
	}
	if a, err := meta.Accessor(obj); err == nil {
		a.SetManagedFields(nil)
		if ann := a.GetAnnotations(); ann != nil {
			delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
		}
	}
	return obj, nil
}

// startWatchCache checks which kinds this identity may list (one LIMIT=1 list
// each — read-only, no SelfSubjectAccessReview "create"), starts informers
// for those, and blocks until each has finished its initial paginated list.
func startWatchCache(ctx context.Context, cs kubernetes.Interface, dyn dynamic.Interface) (*watchCache, error) {
	wc := &watchCache{
		typed:       map[string]cache.SharedIndexInformer{},
		dyn:         map[schema.GroupVersionResource]cache.SharedIndexInformer{},
		denied:      map[string]error{},
		factory:     informers.NewSharedInformerFactory(cs, 0),
		changed:     make(chan struct{}, 1),
		watchDenied: make(chan struct{}),
	}
	probeOpts := metav1.ListOptions{Limit: 1}
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(any) { wc.notify() },
		UpdateFunc: func(oldObj, newObj any) {
			o, e1 := meta.Accessor(oldObj)
			n, e2 := meta.Accessor(newObj)
			if e1 == nil && e2 == nil && o.GetResourceVersion() == n.GetResourceVersion() {
				return // a resync, not a change
			}
			wc.notify()
		},
		DeleteFunc: func(any) { wc.notify() },
	}
	onWatchErr := func(hctx context.Context, r *cache.Reflector, err error) {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			wc.denyOnce.Do(func() {
				slog.Warn("watch", "event", "watch_forbidden", "error", err.Error())
				close(wc.watchDenied)
			})
			return
		}
		cache.DefaultWatchErrorHandler(hctx, r, err)
	}
	var synced []cache.InformerSynced
	for _, k := range watchedKinds() {
		if err := k.probe(ctx, cs, probeOpts); err != nil {
			wc.denied[k.resource] = err
			slog.Warn("watch", "event", "kind_not_watched", "resource", k.resource, "reason", err.Error())
			continue
		}
		gi, err := wc.factory.ForResource(k.gvr)
		if err != nil {
			wc.denied[k.resource] = err
			continue
		}
		inf := gi.Informer()
		_ = inf.SetTransform(stripForCache)
		_ = inf.SetWatchErrorHandlerWithContext(onWatchErr)
		_, _ = inf.AddEventHandler(handler)
		wc.typed[k.resource] = inf
		synced = append(synced, inf.HasSynced)
	}
	if dyn != nil {
		wc.dynFac = dynamicinformer.NewDynamicSharedInformerFactory(dyn, 0)
		for _, gvr := range watchedCRDs {
			if _, err := dyn.Resource(gvr).List(ctx, probeOpts); err != nil {
				continue // CRD not installed (or not readable): treated as absent, exactly as before
			}
			inf := wc.dynFac.ForResource(gvr).Informer()
			_ = inf.SetTransform(stripForCache)
			_ = inf.SetWatchErrorHandlerWithContext(onWatchErr)
			_, _ = inf.AddEventHandler(handler)
			wc.dyn[gvr] = inf
			synced = append(synced, inf.HasSynced)
		}
		wc.dynFac.Start(ctx.Done())
	}
	wc.factory.Start(ctx.Done())
	syncCtx, cancel := context.WithTimeout(ctx, initialSyncTimeout)
	defer cancel()
	go func() {
		select {
		case <-wc.watchDenied:
			cancel()
		case <-syncCtx.Done():
		}
	}()
	if !cache.WaitForCacheSync(syncCtx.Done(), synced...) {
		select {
		case <-wc.watchDenied:
			return nil, errWatchDenied
		default:
		}
		return nil, fmt.Errorf("watch cache did not finish its initial sync within %s", initialSyncTimeout)
	}
	return wc, nil
}

func (wc *watchCache) notify() {
	wc.events.Add(1)
	select {
	case wc.changed <- struct{}{}:
	default:
	}
}

// skippedResources lists the kinds this identity may not list, for the
// payload's meta.skipped (same meaning as in poll mode).
func (wc *watchCache) skippedResources() []string {
	var out []string
	for r, err := range wc.denied {
		if apierrors.IsForbidden(err) {
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}

// items returns typed objects of one resource from the cache, optionally in
// one namespace, as values (copies), so the graph builder can never mutate
// the shared cache.
func items[T any](wc *watchCache, resource, ns string) ([]T, error) {
	inf, ok := wc.typed[resource]
	if !ok {
		if err, denied := wc.denied[resource]; denied && apierrors.IsForbidden(err) {
			return nil, err
		}
		return nil, apierrors.NewForbidden(schema.GroupResource{Resource: resource}, "", fmt.Errorf("not watched"))
	}
	var objs []any
	if ns == "" {
		objs = inf.GetStore().List()
	} else {
		var err error
		if objs, err = inf.GetIndexer().ByIndex(cache.NamespaceIndex, ns); err != nil {
			return nil, err
		}
	}
	out := make([]T, 0, len(objs))
	for _, o := range objs {
		if t, ok := o.(*T); ok {
			out = append(out, *t)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// cachedClientset: a kubernetes.Interface whose List calls (and the one Get
// the builder makes) are answered from the watch cache. Every other method
// falls through to the real clientset, unchanged. graph.BuildClusterGraph is
// handed this instead of the real client, so the relationship logic — and its
// tests — are identical in watch and poll mode.
// ---------------------------------------------------------------------------

type cachedClientset struct {
	kubernetes.Interface
	wc *watchCache
}

func (c cachedClientset) CoreV1() corev1Client     { return cCore{c.Interface.CoreV1(), c.wc} }
func (c cachedClientset) AppsV1() appsv1Client     { return cApps{c.Interface.AppsV1(), c.wc} }
func (c cachedClientset) BatchV1() batchv1Client   { return cBatch{c.Interface.BatchV1(), c.wc} }
func (c cachedClientset) DiscoveryV1() discClient  { return cDisc{c.Interface.DiscoveryV1(), c.wc} }
func (c cachedClientset) NetworkingV1() netClient  { return cNet{c.Interface.NetworkingV1(), c.wc} }
func (c cachedClientset) RbacV1() rbacClient       { return cRbac{c.Interface.RbacV1(), c.wc} }
func (c cachedClientset) StorageV1() storageClient { return cStorage{c.Interface.StorageV1(), c.wc} }
func (c cachedClientset) AutoscalingV2() hpaClient { return cHPA{c.Interface.AutoscalingV2(), c.wc} }

// listOf adapts items[] to a typed List result.
func listOf[T any, L any](wc *watchCache, resource, ns string, wrap func([]T) *L) (*L, error) {
	it, err := items[T](wc, resource, ns)
	if err != nil {
		return nil, err
	}
	return wrap(it), nil
}

// dynamic: the builder reads VPAs and Karpenter NodePools this way.
type cachedDynamic struct {
	dynamic.Interface
	wc *watchCache
}

func (d cachedDynamic) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return cDynRes{NamespaceableResourceInterface: d.Interface.Resource(gvr), wc: d.wc, gvr: gvr}
}

type cDynRes struct {
	dynamic.NamespaceableResourceInterface
	wc  *watchCache
	gvr schema.GroupVersionResource
	ns  string
}

func (r cDynRes) Namespace(ns string) dynamic.ResourceInterface { r.ns = ns; return r }

func (r cDynRes) List(ctx context.Context, o metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	inf, ok := r.wc.dyn[r.gvr]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: r.gvr.Group, Resource: r.gvr.Resource}, "")
	}
	var objs []any
	if r.ns == "" {
		objs = inf.GetStore().List()
	} else {
		objs, _ = inf.GetIndexer().ByIndex(cache.NamespaceIndex, r.ns)
	}
	out := &unstructured.UnstructuredList{}
	for _, o := range objs {
		if u, ok := o.(*unstructured.Unstructured); ok {
			out.Items = append(out.Items, *u.DeepCopy())
		}
	}
	return out, nil
}

// keep the compiler honest about which typed lists the builder relies on.
var (
	_ = (*appsv1.DeploymentList)(nil)
	_ = (*autoscalingv2.HorizontalPodAutoscalerList)(nil)
	_ = (*batchv1.JobList)(nil)
	_ = (*discoveryv1.EndpointSliceList)(nil)
	_ = (*networkingv1.IngressList)(nil)
	_ = (*rbacv1.RoleList)(nil)
	_ = (*storagev1.StorageClassList)(nil)
)
