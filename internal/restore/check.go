package restore

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/snapshot"
)

var (
	gvrNamespace      = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	gvrPod            = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	gvrClaim          = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	gvrReplicaSet     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	gvrVolumeSnapshot = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"}
)

// checkSnapshot makes sure everything the restore needs is there before anything is changed.
func (r *Reconciler) checkSnapshot(ctx context.Context, req *RestoreRequest, env *environment) (bool, error) {
	if req.Spec.Export != "" {
		return r.checkExport(ctx, req, env)
	}
	if err := env.load(ctx, r, req); err != nil {
		return false, err
	}
	if env.snap.Status.Phase != snapshot.PhaseSucceeded {
		return false, &snapshot.Failure{
			Code:    "snapshot_not_ready",
			Message: fmt.Sprintf("Backup %s did not finish successfully.", env.snap.Name),
			Hint:    "Pick a backup that finished successfully.",
		}
	}

	application := req.Spec.Application
	if _, err := r.Client.Resource(gvrNamespace).Get(ctx, application, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return false, &snapshot.Failure{
			Code:    "application_missing",
			Message: fmt.Sprintf("Namespace %s does not exist.", application),
			Hint:    "Restoring into a deleted namespace is not supported yet. Create the namespace again and restore.",
		}
	} else if err != nil {
		return false, fmt.Errorf("get namespace %s: %w", application, err)
	}

	volumes := make([]RestoredVolume, 0, len(env.snap.Status.Volumes))
	for _, v := range env.snap.Status.Volumes {
		if err := r.checkVolumeSnapshot(ctx, application, v); err != nil {
			return false, err
		}
		if findItem(env.bundle, "PersistentVolumeClaim", v.Claim) == nil {
			return false, &snapshot.Failure{
				Code:    "export_missing",
				Message: fmt.Sprintf("The saved resources do not contain volume claim %s.", v.Claim),
				Hint:    "The backup is incomplete and cannot be restored.",
			}
		}
		volumes = append(volumes, RestoredVolume{Claim: v.Claim, VolumeSnapshot: v.VolumeSnapshot})
	}
	if len(req.Status.Volumes) == 0 {
		req.Status.Volumes = volumes
	}
	return true, nil
}

func (r *Reconciler) checkVolumeSnapshot(ctx context.Context, application string, v snapshot.VolumeStatus) error {
	obj, err := r.Client.Resource(gvrVolumeSnapshot).Namespace(application).Get(ctx, v.VolumeSnapshot, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return &snapshot.Failure{
			Code:    "volume_snapshot_missing",
			Message: fmt.Sprintf("VolumeSnapshot %s of volume %s no longer exists.", v.VolumeSnapshot, v.Claim),
			Hint:    "The snapshot was deleted outside of Restora. This backup cannot be restored.",
		}
	}
	if err != nil {
		return fmt.Errorf("get volume snapshot %s: %w", v.VolumeSnapshot, err)
	}
	if ready, _, _ := unstructured.NestedBool(obj.Object, "status", "readyToUse"); !ready {
		return &snapshot.Failure{
			Code:    "volume_snapshot_missing",
			Message: fmt.Sprintf("VolumeSnapshot %s of volume %s is not usable.", v.VolumeSnapshot, v.Claim),
			Hint:    "Check the storage driver. This backup cannot be restored right now.",
		}
	}
	return nil
}

// findItem returns the exported object of a kind and name, or nil.
func findItem(b snapshot.Bundle, kind, name string) map[string]any {
	for _, item := range b.Items {
		if item["kind"] != kind {
			continue
		}
		if meta, ok := item["metadata"].(map[string]any); ok && meta["name"] == name {
			return item
		}
	}
	return nil
}

// checkExport prepares a restore from a storage location: it fetches the saved resources from the repository, makes
// sure the namespace exists (it may have been deleted), and lists the volumes to fill.
func (r *Reconciler) checkExport(ctx context.Context, req *RestoreRequest, env *environment) (bool, error) {
	record, err := r.Records.Get(ctx, req.Spec.Export)
	if errors.Is(err, offsite.ErrNotFound) {
		return false, &snapshot.Failure{
			Code:    "export_not_found",
			Message: fmt.Sprintf("The stored backup %s does not exist any more.", req.Spec.Export),
			Hint:    "Pick another backup.",
		}
	}
	if err != nil {
		return false, err
	}
	if r.Offsite == nil {
		return false, &snapshot.Failure{Code: "export_unavailable", Message: "This version of Restora cannot restore from storage locations.", Hint: "Update the Restora controller."}
	}
	if ready, err := r.Offsite.Prepare(ctx, req.Name, record); err != nil || !ready {
		return false, err
	}
	if err := env.load(ctx, r, req); err != nil {
		return false, err
	}

	manifest, err := r.Offsite.Manifest(req.Name)
	if err != nil {
		return false, &snapshot.Failure{Code: "export_missing", Message: fmt.Sprintf("The manifest of the stored backup cannot be read: %v", err),
			Hint: "Start the restore again."}
	}
	if err := r.ensureNamespace(ctx, req.Spec.Application, manifest); err != nil {
		return false, err
	}

	volumes := make([]RestoredVolume, 0, len(record.Spec.Volumes))
	for _, v := range record.Spec.Volumes {
		if findItem(env.bundle, "PersistentVolumeClaim", v.Claim) == nil {
			return false, &snapshot.Failure{
				Code:    "export_missing",
				Message: fmt.Sprintf("The saved resources do not contain volume claim %s.", v.Claim),
				Hint:    "The stored backup is incomplete and cannot be restored.",
			}
		}
		volumes = append(volumes, RestoredVolume{Claim: v.Claim})
	}
	if len(req.Status.Volumes) == 0 {
		req.Status.Volumes = volumes
	}
	return true, nil
}

// ensureNamespace creates the namespace of the application when it was deleted, with the labels and annotations it
// had.
func (r *Reconciler) ensureNamespace(ctx context.Context, application string, manifest offsite.Manifest) error {
	_, err := r.Client.Resource(gvrNamespace).Get(ctx, application, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get namespace %s: %w", application, err)
	}
	ns := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace"}}
	ns.SetName(application)
	labels := map[string]string{}
	for k, v := range manifest.Namespace.Labels {
		labels[k] = v
	}
	labels[snapshot.LabelManagedBy] = snapshot.ManagedByValue
	ns.SetLabels(labels)
	if len(manifest.Namespace.Annotations) > 0 {
		ns.SetAnnotations(manifest.Namespace.Annotations)
	}
	if _, err := r.Client.Resource(gvrNamespace).Create(ctx, ns, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", application, err)
	}
	r.Log.Info("namespace created for the restore", "namespace", application)
	return nil
}
