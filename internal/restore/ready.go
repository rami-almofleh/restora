package restore

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"restora.local/restora/internal/snapshot"
)

// waitReady waits until the restored workloads have all their replicas ready, so "done" means the application runs.
// If they do not start in time, the global timeout fails the restore with workloads_not_ready.
func (r *Reconciler) waitReady(ctx context.Context, req *RestoreRequest, env *environment) (bool, error) {
	if err := env.load(ctx, r, req); err != nil {
		return false, err
	}

	wanted := map[workloadRef]int32{}
	for _, item := range env.bundle.Items {
		kind, _ := item["kind"].(string)
		if kind != "Deployment" && kind != "StatefulSet" {
			continue
		}
		obj := &unstructured.Unstructured{Object: item}
		wanted[workloadRef{kind, obj.GetName()}] = desiredReplicas(obj)
	}
	for _, s := range req.Status.Stopped {
		// A workload from the backup keeps the size of the backup. Only the others return to their old size.
		if _, inBackup := wanted[workloadRef{s.Kind, s.Name}]; !inBackup {
			wanted[workloadRef{s.Kind, s.Name}] = s.Replicas
		}
	}

	for ref, replicas := range wanted {
		if replicas == 0 {
			continue
		}
		gvr, _ := snapshot.TargetGVR(ref.kind)
		obj, err := r.Client.Resource(gvr).Namespace(req.Spec.Application).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("get %s %s: %w", ref.kind, ref.name, err)
		}
		ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
		if int32(ready) < replicas {
			return false, nil
		}
	}
	return true, nil
}
