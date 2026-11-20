package snapshot

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	gvrNamespace = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	gvrClaim     = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}
	gvrVolume    = schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"}

	gvrVolumeSnapshot      = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshots"}
	gvrVolumeSnapshotClass = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotclasses"}
)

const defaultClassAnnotation = "snapshot.storage.kubernetes.io/is-default-class"

// Volumes creates and watches CSI VolumeSnapshots.
type Volumes struct {
	client dynamic.Interface
}

// NewVolumes creates a Volumes helper.
func NewVolumes(client dynamic.Interface) *Volumes {
	return &Volumes{client: client}
}

// PlannedVolume is one claim together with the snapshot class that will be used for it.
type PlannedVolume struct {
	Claim     string
	ClassName string
}

// Plan checks every claim of the application and picks a snapshot class for each.
// It changes nothing, so a problem is found before the first VolumeSnapshot is created.
func (v *Volumes) Plan(ctx context.Context, application, classOverride string) ([]PlannedVolume, error) {
	if _, err := v.client.Resource(gvrNamespace).Get(ctx, application, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return nil, &Failure{
			Code:    "application_missing",
			Message: fmt.Sprintf("Namespace %s does not exist.", application),
			Hint:    "The application was deleted before the backup started. Pick an existing application.",
		}
	} else if err != nil {
		return nil, fmt.Errorf("get namespace %s: %w", application, err)
	}

	claims, err := v.client.Resource(gvrClaim).Namespace(application).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list volume claims in %s: %w", application, err)
	}
	classes, err := v.classesByDriver(ctx)
	if err != nil {
		return nil, err
	}

	planned := make([]PlannedVolume, 0, len(claims.Items))
	for _, claim := range claims.Items {
		driver, err := v.driverOf(ctx, &claim)
		if err != nil {
			return nil, err
		}
		class, err := pickClass(classes, driver, classOverride)
		if err != nil {
			return nil, err
		}
		planned = append(planned, PlannedVolume{Claim: claim.GetName(), ClassName: class})
	}
	return planned, nil
}

// driverOf returns the CSI driver behind a claim or a Failure when it cannot be snapshotted.
func (v *Volumes) driverOf(ctx context.Context, claim *unstructured.Unstructured) (string, error) {
	phase, _, _ := unstructured.NestedString(claim.Object, "status", "phase")
	volumeName, _, _ := unstructured.NestedString(claim.Object, "spec", "volumeName")
	if phase != "Bound" || volumeName == "" {
		return "", &Failure{
			Code:    "volume_not_bound",
			Message: fmt.Sprintf("Volume %s is not bound to storage yet.", claim.GetName()),
			Hint:    "Wait until the volume is bound (its pod must be scheduled if the storage class waits for a consumer), then try again.",
		}
	}

	pv, err := v.client.Resource(gvrVolume).Get(ctx, volumeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get persistent volume %s: %w", volumeName, err)
	}
	driver, found, _ := unstructured.NestedString(pv.Object, "spec", "csi", "driver")
	if !found || driver == "" {
		return "", &Failure{
			Code:    "volume_not_snapshottable",
			Message: fmt.Sprintf("Volume %s does not use a CSI driver, so it cannot be snapshotted.", claim.GetName()),
			Hint:    "Only volumes from a CSI storage class support snapshots. Move the data to a CSI storage class.",
		}
	}
	return driver, nil
}

type snapshotClass struct {
	name      string
	isDefault bool
}

// classesByDriver groups the VolumeSnapshotClasses by CSI driver. Within a driver the default class comes
// first, then the rest by name.
func (v *Volumes) classesByDriver(ctx context.Context) (map[string][]snapshotClass, error) {
	list, err := v.client.Resource(gvrVolumeSnapshotClass).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string][]snapshotClass{}, nil // the snapshot CRDs are not installed
		}
		return nil, fmt.Errorf("list volume snapshot classes: %w", err)
	}
	byDriver := map[string][]snapshotClass{}
	for _, item := range list.Items {
		driver, _, _ := unstructured.NestedString(item.Object, "driver")
		byDriver[driver] = append(byDriver[driver], snapshotClass{
			name:      item.GetName(),
			isDefault: item.GetAnnotations()[defaultClassAnnotation] == "true",
		})
	}
	for _, classes := range byDriver {
		sort.Slice(classes, func(i, j int) bool {
			if classes[i].isDefault != classes[j].isDefault {
				return classes[i].isDefault
			}
			return classes[i].name < classes[j].name
		})
	}
	return byDriver, nil
}

func pickClass(byDriver map[string][]snapshotClass, driver, override string) (string, error) {
	classes := byDriver[driver]
	if override != "" {
		for _, c := range classes {
			if c.name == override {
				return c.name, nil
			}
		}
		return "", &Failure{
			Code:    "snapshot_class_missing",
			Message: fmt.Sprintf("VolumeSnapshotClass %s does not exist for the CSI driver %s.", override, driver),
			Hint:    "Create the class, or remove the class name from the snapshot to let Restora choose one.",
		}
	}
	if len(classes) == 0 {
		return "", &Failure{
			Code:    "snapshot_class_missing",
			Message: fmt.Sprintf("No VolumeSnapshotClass exists for the CSI driver %s.", driver),
			Hint:    "Create a VolumeSnapshotClass for this driver (see your storage provider's documentation) and try again.",
		}
	}
	return classes[0].name, nil
}

// Create makes one VolumeSnapshot per planned claim and returns their names. It is safe to call again:
// snapshots that exist already are kept.
func (v *Volumes) Create(ctx context.Context, snapshotName, application string, planned []PlannedVolume) ([]VolumeStatus, error) {
	out := make([]VolumeStatus, 0, len(planned))
	for i, p := range planned {
		name := snapshotName + "-" + strconv.Itoa(i)
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": gvrVolumeSnapshot.Group + "/" + gvrVolumeSnapshot.Version,
			"kind":       "VolumeSnapshot",
			"metadata": map[string]any{
				"name":      name,
				"namespace": application,
				"labels": map[string]any{
					LabelManagedBy:   ManagedByValue,
					LabelApplication: application,
					LabelSnapshot:    snapshotName,
				},
			},
			"spec": map[string]any{
				"volumeSnapshotClassName": p.ClassName,
				"source":                  map[string]any{"persistentVolumeClaimName": p.Claim},
			},
		}}
		_, err := v.client.Resource(gvrVolumeSnapshot).Namespace(application).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create volume snapshot %s: %w", name, err)
		}
		out = append(out, VolumeStatus{Claim: p.Claim, VolumeSnapshot: name, ClassName: p.ClassName})
	}
	return out, nil
}

// Refresh reads the state of each VolumeSnapshot into the volume list.
func (v *Volumes) Refresh(ctx context.Context, application string, volumes []VolumeStatus) ([]VolumeStatus, error) {
	out := make([]VolumeStatus, len(volumes))
	for i, vol := range volumes {
		out[i] = vol
		obj, err := v.client.Resource(gvrVolumeSnapshot).Namespace(application).Get(ctx, vol.VolumeSnapshot, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil, &Failure{
				Code:    "snapshot_failed",
				Message: fmt.Sprintf("VolumeSnapshot %s disappeared during the backup.", vol.VolumeSnapshot),
				Hint:    "Someone deleted it or the application namespace was removed. Start a new backup.",
			}
		}
		if err != nil {
			return nil, fmt.Errorf("get volume snapshot %s: %w", vol.VolumeSnapshot, err)
		}
		out[i].Ready, _, _ = unstructured.NestedBool(obj.Object, "status", "readyToUse")
		out[i].Error, _, _ = unstructured.NestedString(obj.Object, "status", "error", "message")
		if out[i].Ready {
			out[i].Error = ""
			out[i].SizeBytes = restoreSize(obj)
		}
	}
	return out, nil
}

// restoreSize reads status.restoreSize, which the API serialises as a quantity such as "1Gi".
func restoreSize(obj *unstructured.Unstructured) int64 {
	raw, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "status", "restoreSize")
	if !found {
		return 0
	}
	switch value := raw.(type) {
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		if q, err := resource.ParseQuantity(value); err == nil {
			return q.Value()
		}
	}
	return 0
}

// Delete removes all VolumeSnapshots that belong to a snapshot. A missing namespace is not an error.
func (v *Volumes) Delete(ctx context.Context, snapshotName, application string) error {
	client := v.client.Resource(gvrVolumeSnapshot).Namespace(application)
	list, err := client.List(ctx, metav1.ListOptions{LabelSelector: LabelSnapshot + "=" + snapshotName})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("list volume snapshots of %s: %w", snapshotName, err)
	}
	for _, item := range list.Items {
		if err := client.Delete(ctx, item.GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete volume snapshot %s: %w", item.GetName(), err)
		}
	}
	return nil
}
