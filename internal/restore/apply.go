package restore

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"restora.local/restora/internal/snapshot"
)

// restoreResources applies the saved resources, except the volume claims that restoreVolumes handled.
// A saved resource replaces a live one of the same name. Resources that exist only now stay as they are.
func (r *Reconciler) restoreResources(ctx context.Context, req *RestoreRequest, env *environment) (bool, error) {
	if err := env.load(ctx, r, req); err != nil {
		return false, err
	}
	result := ResourceResult{}
	req.Status.Warnings = nil

	for _, item := range env.bundle.Items {
		kind, _ := item["kind"].(string)
		if kind == "PersistentVolumeClaim" {
			continue
		}
		outcome, err := r.apply(ctx, req.Spec.Application, kind, item)
		if err != nil {
			return false, err
		}
		switch outcome.action {
		case actionCreated:
			result.Created++
		case actionUpdated:
			result.Updated++
		case actionSkipped:
			result.Skipped++
			req.Status.Warnings = append(req.Status.Warnings, outcome.reason)
		}
	}
	req.Status.Resources = &result

	// Workloads that did not exist at backup time were only stopped. Start them again.
	err := r.scaleBack(ctx, req, func(s StoppedWorkload) bool {
		return findItem(env.bundle, s.Kind, s.Name) != nil
	})
	return err == nil, err
}

type action string

const (
	actionCreated action = "created"
	actionUpdated action = "updated"
	actionSkipped action = "skipped"
)

type outcome struct {
	action action
	reason string
}

// apply creates or replaces one saved resource.
func (r *Reconciler) apply(ctx context.Context, namespace, kind string, saved map[string]any) (outcome, error) {
	gvr, ok := snapshot.TargetGVR(kind)
	if !ok {
		return outcome{actionSkipped, fmt.Sprintf("%s: kind is not supported by this version", kind)}, nil
	}
	obj := (&unstructured.Unstructured{Object: saved}).DeepCopy()
	obj.SetNamespace(namespace)
	name := obj.GetName()
	label := kind + "/" + name
	client := r.Client.Resource(gvr).Namespace(namespace)

	existing, err := client.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			if skip, reason := tolerated(kind, label, err); skip {
				return outcome{actionSkipped, reason}, nil
			}
			return outcome{}, fmt.Errorf("create %s: %w", label, err)
		}
		return outcome{action: actionCreated}, nil
	}
	if err != nil {
		return outcome{}, fmt.Errorf("get %s: %w", label, err)
	}

	// A Job's spec cannot be changed, and a finished Job has nothing to restore.
	if kind == "Job" {
		return outcome{actionSkipped, label + ": already exists and Jobs cannot be changed"}, nil
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	if kind == "Service" {
		keepServiceAddress(existing, obj)
	}
	if _, err := client.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsInvalid(err) {
			// For example a field that Kubernetes does not allow to change. Keep the live object and say so.
			return outcome{actionSkipped, fmt.Sprintf("%s: kept as it is (%v)", label, err)}, nil
		}
		if skip, reason := tolerated(kind, label, err); skip {
			return outcome{actionSkipped, reason}, nil
		}
		return outcome{}, fmt.Errorf("update %s: %w", label, err)
	}
	return outcome{action: actionUpdated}, nil
}

// tolerated reports errors that must not fail a restore: Kubernetes only lets the controller write roles and
// bindings for permissions it holds itself, so those are reported as skipped.
func tolerated(kind, label string, err error) (bool, string) {
	if apierrors.IsForbidden(err) && (kind == "Role" || kind == "RoleBinding") {
		return true, fmt.Sprintf("%s: not restored, Restora may not grant these permissions (%v)", label, err)
	}
	return false, ""
}

// keepServiceAddress copies the cluster IPs of the live Service: they cannot be changed, and the saved ones may be taken.
func keepServiceAddress(live, saved *unstructured.Unstructured) {
	for _, field := range []string{"clusterIP", "clusterIPs"} {
		if v, found, _ := unstructured.NestedFieldCopy(live.Object, "spec", field); found {
			_ = unstructured.SetNestedField(saved.Object, v, "spec", field)
		}
	}
}
