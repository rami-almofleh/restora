package restore

import (
	"context"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"restora.local/restora/internal/snapshot"
)

type workloadRef struct{ kind, name string }

// stopWorkloads scales down every Deployment and StatefulSet whose pods use a volume that will be replaced.
// A volume cannot be deleted while a pod uses it.
func (r *Reconciler) stopWorkloads(ctx context.Context, req *RestoreRequest, _ *environment) (bool, error) {
	application := req.Spec.Application
	claims := make([]string, 0, len(req.Status.Volumes))
	for _, v := range req.Status.Volumes {
		claims = append(claims, v.Claim)
	}

	pods, err := r.Client.Resource(gvrPod).Namespace(application).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("list pods in %s: %w", application, err)
	}
	var targets []workloadRef
	using := 0
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !mountsAny(pod, claims) {
			continue
		}
		using++
		ref, err := r.ownerOf(ctx, application, pod)
		if err != nil {
			return false, err
		}
		if !slices.Contains(targets, ref) {
			targets = append(targets, ref)
		}
	}

	// Write down the replica counts before scaling, so a restart of the controller cannot lose them.
	recorded := false
	for _, ref := range targets {
		if slices.ContainsFunc(req.Status.Stopped, func(s StoppedWorkload) bool { return s.Kind == ref.kind && s.Name == ref.name }) {
			continue
		}
		replicas, err := r.replicas(ctx, application, ref)
		if err != nil {
			return false, err
		}
		req.Status.Stopped = append(req.Status.Stopped, StoppedWorkload{Kind: ref.kind, Name: ref.name, Replicas: replicas})
		recorded = true
	}
	if recorded {
		if err := r.save(ctx, req); err != nil {
			return false, err
		}
	}
	for _, ref := range targets {
		if err := r.scale(ctx, application, ref, 0); err != nil {
			return false, err
		}
	}
	return using == 0, nil
}

// scaleBack returns stopped workloads to their recorded size. Workloads named in skip are left alone,
// because the saved resources set their size.
func (r *Reconciler) scaleBack(ctx context.Context, req *RestoreRequest, skip func(StoppedWorkload) bool) error {
	for _, s := range req.Status.Stopped {
		if skip != nil && skip(s) {
			continue
		}
		if err := r.scale(ctx, req.Spec.Application, workloadRef{s.Kind, s.Name}, s.Replicas); err != nil {
			return err
		}
	}
	return nil
}

// ownerOf finds the Deployment or StatefulSet that runs a pod. Anything else (a bare pod, a Job, a DaemonSet)
// would come back on its own or not at all, so the restore refuses to touch it.
func (r *Reconciler) ownerOf(ctx context.Context, namespace string, pod *unstructured.Unstructured) (workloadRef, error) {
	unmanaged := &snapshot.Failure{
		Code:    "volume_in_use",
		Message: fmt.Sprintf("Pod %s uses a volume that must be replaced, but no Deployment or StatefulSet manages it.", pod.GetName()),
		Hint:    "Stop or delete this pod yourself, then start the restore again.",
	}
	owner := controllerOf(pod.GetOwnerReferences())
	if owner == nil {
		return workloadRef{}, unmanaged
	}
	switch owner.Kind {
	case "StatefulSet":
		return workloadRef{"StatefulSet", owner.Name}, nil
	case "ReplicaSet":
		rs, err := r.Client.Resource(gvrReplicaSet).Namespace(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return workloadRef{}, unmanaged
		}
		if err != nil {
			return workloadRef{}, fmt.Errorf("get replica set %s: %w", owner.Name, err)
		}
		if deployment := controllerOf(rs.GetOwnerReferences()); deployment != nil && deployment.Kind == "Deployment" {
			return workloadRef{"Deployment", deployment.Name}, nil
		}
	}
	return workloadRef{}, unmanaged
}

func controllerOf(refs []metav1.OwnerReference) *metav1.OwnerReference {
	for i := range refs {
		if refs[i].Controller != nil && *refs[i].Controller {
			return &refs[i]
		}
	}
	return nil
}

// mountsAny reports whether a running pod uses one of the claims.
func mountsAny(pod *unstructured.Unstructured, claims []string) bool {
	if phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase"); phase == "Succeeded" || phase == "Failed" {
		return false // finished pods no longer hold their volumes
	}
	volumes, _, _ := unstructured.NestedSlice(pod.Object, "spec", "volumes")
	for _, v := range volumes {
		volume, _ := v.(map[string]any)
		claim, _, _ := unstructured.NestedString(volume, "persistentVolumeClaim", "claimName")
		if claim != "" && slices.Contains(claims, claim) {
			return true
		}
	}
	return false
}

func (r *Reconciler) workload(ctx context.Context, namespace string, ref workloadRef) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	gvr, ok := snapshot.TargetGVR(ref.kind)
	if !ok {
		return nil, gvr, fmt.Errorf("unknown workload kind %s", ref.kind)
	}
	obj, err := r.Client.Resource(gvr).Namespace(namespace).Get(ctx, ref.name, metav1.GetOptions{})
	return obj, gvr, err
}

// replicas returns the desired replica count of a workload, with the Kubernetes default of one.
func (r *Reconciler) replicas(ctx context.Context, namespace string, ref workloadRef) (int32, error) {
	obj, _, err := r.workload(ctx, namespace, ref)
	if err != nil {
		return 0, fmt.Errorf("get %s %s: %w", ref.kind, ref.name, err)
	}
	return desiredReplicas(obj), nil
}

func desiredReplicas(obj *unstructured.Unstructured) int32 {
	n, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	if !found {
		return 1
	}
	return int32(n)
}

// scale sets the replica count of a workload. A workload that is gone is not an error.
func (r *Reconciler) scale(ctx context.Context, namespace string, ref workloadRef, replicas int32) error {
	obj, gvr, err := r.workload(ctx, namespace, ref)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get %s %s: %w", ref.kind, ref.name, err)
	}
	if desiredReplicas(obj) == replicas {
		return nil
	}
	if err := unstructured.SetNestedField(obj.Object, int64(replicas), "spec", "replicas"); err != nil {
		return fmt.Errorf("set replicas of %s %s: %w", ref.kind, ref.name, err)
	}
	if _, err := r.Client.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("scale %s %s: %w", ref.kind, ref.name, err)
	}
	return nil
}
