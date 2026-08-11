// Package history reconstructs the revision history that Kubernetes retains for
// workloads. It reads only what the API keeps — for a Deployment, that is the
// ReplicaSets it owns, each tagged with a revision number — and diffs
// consecutive pod templates to describe what changed.
//
// It reports facts the API holds (what changed, and when a revision appeared),
// never who made a change or why: the core API does not record that. History is
// also bounded by the Deployment's revisionHistoryLimit — older revisions are
// garbage-collected and cannot be recovered.
package history

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	revisionAnnotation    = "deployment.kubernetes.io/revision"
	changeCauseAnnotation = "kubernetes.io/change-cause"
)

// Revision is a single point in a workload's rollout history.
type Revision struct {
	Number int64
	Time   time.Time
	// ChangeCause is the recorded kubernetes.io/change-cause, if any.
	ChangeCause string
	// Changes describes what this revision changed relative to the previous one,
	// as human-readable lines. Empty for the initial revision.
	Changes []string
	// Initial is true for the earliest retained revision, which has nothing to
	// diff against.
	Initial bool
}

// DeploymentRevisions returns the retained revisions of a Deployment, oldest
// first, reconstructed from the ReplicaSets it owns.
func DeploymentRevisions(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	name string,
) ([]Revision, error) {
	deployment, err := clientset.AppsV1().
		Deployments(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %q: %w", name, err)
	}

	rsList, err := clientset.AppsV1().
		ReplicaSets(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list replicasets: %w", err)
	}

	type owned struct {
		number int64
		rs     *appsv1.ReplicaSet
	}

	var revisions []owned
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if !controlledBy(rs, deployment.UID) {
			continue
		}
		raw := rs.Annotations[revisionAnnotation]
		if raw == "" {
			continue
		}
		number, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		revisions = append(revisions, owned{number: number, rs: rs})
	}

	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].number < revisions[j].number
	})

	result := make([]Revision, 0, len(revisions))
	var prev *appsv1.ReplicaSet
	for _, o := range revisions {
		rev := Revision{
			Number:      o.number,
			Time:        o.rs.CreationTimestamp.Time,
			ChangeCause: o.rs.Annotations[changeCauseAnnotation],
		}
		if prev == nil {
			rev.Initial = true
		} else {
			rev.Changes = diffTemplates(&prev.Spec.Template, &o.rs.Spec.Template)
		}
		result = append(result, rev)
		prev = o.rs
	}

	return result, nil
}

// controlledBy reports whether rs is controlled by the object with the given UID.
func controlledBy(rs *appsv1.ReplicaSet, uid types.UID) bool {
	for _, owner := range rs.OwnerReferences {
		if owner.UID == uid && owner.Controller != nil && *owner.Controller {
			return true
		}
	}
	return false
}

// diffTemplates describes how the current pod template differs from the previous
// one, focusing on the changes engineers most often care about (container images
// and environment), with a catch-all for anything else in the pod spec.
func diffTemplates(prev, cur *corev1.PodTemplateSpec) []string {
	var changes []string
	changes = append(changes, diffContainers("container", prev.Spec.Containers, cur.Spec.Containers)...)
	changes = append(changes, diffContainers("init container", prev.Spec.InitContainers, cur.Spec.InitContainers)...)

	if len(changes) == 0 && !reflect.DeepEqual(prev.Spec, cur.Spec) {
		changes = append(changes, "pod template changed")
	}

	return changes
}

func diffContainers(kind string, prev, cur []corev1.Container) []string {
	prevByName := indexContainers(prev)
	curByName := indexContainers(cur)

	var changes []string
	for _, name := range unionKeys(prevByName, curByName) {
		p, hasP := prevByName[name]
		c, hasC := curByName[name]

		switch {
		case hasP && !hasC:
			changes = append(changes, fmt.Sprintf("%s %s removed", kind, name))
		case !hasP && hasC:
			changes = append(changes, fmt.Sprintf("%s %s added (image %s)", kind, name, c.Image))
		default:
			if p.Image != c.Image {
				changes = append(changes, fmt.Sprintf("%s %s image: %s → %s", kind, name, p.Image, c.Image))
			}
			changes = append(changes, diffEnv(p.Env, c.Env)...)
		}
	}
	return changes
}

func diffEnv(prev, cur []corev1.EnvVar) []string {
	prevByName := indexEnv(prev)
	curByName := indexEnv(cur)

	var changes []string
	for _, name := range unionEnvKeys(prevByName, curByName) {
		p, hasP := prevByName[name]
		c, hasC := curByName[name]

		switch {
		case hasP && !hasC:
			changes = append(changes, fmt.Sprintf("env %s removed (%s)", name, p))
		case !hasP && hasC:
			changes = append(changes, fmt.Sprintf("env %s added (%s)", name, c))
		case p != c:
			changes = append(changes, fmt.Sprintf("env %s: %s → %s", name, p, c))
		}
	}
	return changes
}

func indexContainers(cs []corev1.Container) map[string]corev1.Container {
	m := make(map[string]corev1.Container, len(cs))
	for _, c := range cs {
		m[c.Name] = c
	}
	return m
}

func indexEnv(envs []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		m[e.Name] = envValue(e)
	}
	return m
}

// envValue renders an environment variable's value or a short description of the
// source it is drawn from.
func envValue(e corev1.EnvVar) string {
	if e.ValueFrom != nil {
		switch {
		case e.ValueFrom.ConfigMapKeyRef != nil:
			return fmt.Sprintf("configMap:%s/%s", e.ValueFrom.ConfigMapKeyRef.Name, e.ValueFrom.ConfigMapKeyRef.Key)
		case e.ValueFrom.SecretKeyRef != nil:
			return fmt.Sprintf("secret:%s/%s", e.ValueFrom.SecretKeyRef.Name, e.ValueFrom.SecretKeyRef.Key)
		case e.ValueFrom.FieldRef != nil:
			return fmt.Sprintf("field:%s", e.ValueFrom.FieldRef.FieldPath)
		case e.ValueFrom.ResourceFieldRef != nil:
			return fmt.Sprintf("resource:%s", e.ValueFrom.ResourceFieldRef.Resource)
		}
		return "valueFrom"
	}
	return e.Value
}

func unionKeys(a, b map[string]corev1.Container) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	return sortedKeys(set)
}

func unionEnvKeys(a, b map[string]string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		set[k] = struct{}{}
	}
	for k := range b {
		set[k] = struct{}{}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
