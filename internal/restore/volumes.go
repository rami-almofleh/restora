package restore

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/snapshot"
)

// restoreVolumes replaces each volume claim by a new one that starts from the VolumeSnapshot.
// A claim cannot change its data source, so the old one is deleted first. This is where data written since
// the backup is lost.
func (r *Reconciler) restoreVolumes(ctx context.Context, req *RestoreRequest, env *environment) (bool, error) {
	if err := env.load(ctx, r, req); err != nil {
		return false, err
	}
	if req.Spec.Export != "" {
		return r.restoreVolumesFromExport(ctx, req, env)
	}
	claims := r.Client.Resource(gvrClaim).Namespace(req.Spec.Application)

	allRestored := true
	changed := false
	for i := range req.Status.Volumes {
		vol := &req.Status.Volumes[i]
		if vol.Restored {
			continue
		}
		existing, err := claims.Get(ctx, vol.Claim, metav1.GetOptions{})
		switch {
		case err == nil && existing.GetAnnotations()[AnnotationRestoredFrom] == vol.VolumeSnapshot:
			vol.Restored, changed = true, true // created by an earlier pass
			continue
		case err == nil:
			if existing.GetDeletionTimestamp() == nil {
				if err := claims.Delete(ctx, vol.Claim, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
					return false, fmt.Errorf("delete volume claim %s: %w", vol.Claim, err)
				}
			}
			allRestored = false // wait until it is gone
			continue
		case !apierrors.IsNotFound(err):
			return false, fmt.Errorf("get volume claim %s: %w", vol.Claim, err)
		}

		template := findItem(env.bundle, "PersistentVolumeClaim", vol.Claim)
		if template == nil {
			return false, &snapshot.Failure{
				Code:    "export_missing",
				Message: fmt.Sprintf("The saved resources do not contain volume claim %s.", vol.Claim),
				Hint:    "The backup is incomplete and cannot be restored.",
			}
		}
		minBytes := sizeOf(env.snap, vol.Claim)
		claim := claimFromSnapshot(template, req.Spec.Application, vol.VolumeSnapshot, minBytes)
		if _, err := claims.Create(ctx, claim, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return false, fmt.Errorf("create volume claim %s: %w", vol.Claim, err)
		}
		vol.Restored, changed = true, true
	}
	if changed {
		if err := r.save(ctx, req); err != nil {
			return false, err
		}
	}
	return allRestored, nil
}

func sizeOf(snap *snapshot.Snapshot, claim string) int64 {
	for _, v := range snap.Status.Volumes {
		if v.Claim == claim {
			return v.SizeBytes
		}
	}
	return 0
}

// runtimeAnnotationPrefixes are set by Kubernetes while a claim is bound. They must not be copied to a new claim.
var runtimeAnnotationPrefixes = []string{"pv.kubernetes.io/", "volume.kubernetes.io/", "volume.beta.kubernetes.io/"}

// claimFromSnapshot builds a new claim from the saved one: same name, class and access modes,
// but empty of any old binding and filled from the VolumeSnapshot. The size is never smaller than the snapshot.
func claimFromSnapshot(saved map[string]any, namespace, volumeSnapshot string, minBytes int64) *unstructured.Unstructured {
	claim := (&unstructured.Unstructured{Object: saved}).DeepCopy()
	claim.SetNamespace(namespace)
	claim.SetFinalizers(nil)
	claim.SetOwnerReferences(nil)

	annotations := map[string]string{}
	for k, v := range claim.GetAnnotations() {
		if !hasAnyPrefix(k, runtimeAnnotationPrefixes) {
			annotations[k] = v
		}
	}
	annotations[AnnotationRestoredFrom] = volumeSnapshot
	claim.SetAnnotations(annotations)

	labels := claim.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[snapshot.LabelManagedBy] = snapshot.ManagedByValue
	claim.SetLabels(labels)

	unstructured.RemoveNestedField(claim.Object, "spec", "volumeName")
	unstructured.RemoveNestedField(claim.Object, "spec", "dataSourceRef")
	_ = unstructured.SetNestedStringMap(claim.Object, map[string]string{
		"apiGroup": gvrVolumeSnapshot.Group,
		"kind":     "VolumeSnapshot",
		"name":     volumeSnapshot,
	}, "spec", "dataSource")

	requested, _, _ := unstructured.NestedString(claim.Object, "spec", "resources", "requests", "storage")
	if q, err := resource.ParseQuantity(requested); err != nil || q.Value() < minBytes {
		_ = unstructured.SetNestedField(claim.Object, resource.NewQuantity(minBytes, resource.BinarySI).String(),
			"spec", "resources", "requests", "storage")
	}
	return claim
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// restoreVolumesFromExport replaces each volume claim by a new, empty one and fills it from the repository with a
// data mover. One volume is worked on at a time. As with a restore from a snapshot, this is where data written
// since the backup is lost.
func (r *Reconciler) restoreVolumesFromExport(ctx context.Context, req *RestoreRequest, env *environment) (bool, error) {
	claims := r.Client.Resource(gvrClaim).Namespace(req.Spec.Application)
	marker := "export:" + req.Spec.Export

	for i := range req.Status.Volumes {
		vol := &req.Status.Volumes[i]
		if vol.Restored {
			continue
		}
		existing, err := claims.Get(ctx, vol.Claim, metav1.GetOptions{})
		switch {
		case err == nil && existing.GetAnnotations()[AnnotationRestoredFrom] == marker:
			// The claim of this restore exists: let the data mover fill it.
			done, err := r.Offsite.RestoreVolume(ctx, req.Name, env.record, recordIndex(env.record, vol.Claim))
			if err != nil {
				return false, err
			}
			if !done {
				return false, r.save(ctx, req)
			}
			vol.Restored = true
			if err := r.save(ctx, req); err != nil {
				return false, err
			}
			continue
		case err == nil:
			if existing.GetDeletionTimestamp() == nil {
				if err := claims.Delete(ctx, vol.Claim, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
					return false, fmt.Errorf("delete volume claim %s: %w", vol.Claim, err)
				}
			}
			return false, nil // wait until it is gone
		case !apierrors.IsNotFound(err):
			return false, fmt.Errorf("get volume claim %s: %w", vol.Claim, err)
		}

		template := findItem(env.bundle, "PersistentVolumeClaim", vol.Claim)
		if template == nil {
			return false, &snapshot.Failure{
				Code:    "export_missing",
				Message: fmt.Sprintf("The saved resources do not contain volume claim %s.", vol.Claim),
				Hint:    "The stored backup is incomplete and cannot be restored.",
			}
		}
		claim := claimFromSnapshot(template, req.Spec.Application, marker, env.record.Spec.Volumes[recordIndex(env.record, vol.Claim)].SizeBytes)
		unstructured.RemoveNestedField(claim.Object, "spec", "dataSource")
		if _, err := claims.Create(ctx, claim, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return false, fmt.Errorf("create volume claim %s: %w", vol.Claim, err)
		}
		return false, nil
	}
	return true, nil
}

// recordIndex is the position of a claim among the volumes of an export.
func recordIndex(rec *offsite.Record, claim string) int {
	for i, v := range rec.Spec.Volumes {
		if v.Claim == claim {
			return i
		}
	}
	return 0
}
