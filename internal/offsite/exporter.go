package offsite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"log/slog"

	"restora.local/restora/internal/location"
	"restora.local/restora/internal/snapshot"
)

const (
	// maxBundleBytes is the largest set of saved resources that can go through a secret (the limit is 1 MiB).
	maxBundleBytes = 900 * 1024
	// startGrace is how long a data mover pod may stay without starting a container before the copy fails.
	startGrace = 5 * time.Minute

	manifestFile = "manifest.json"
	bundleFile   = "resources.json.gz"
)

// Manifest is stored next to the saved resources in the repository, so a repository describes itself.
type Manifest struct {
	Version     int       `json:"version"`
	Application string    `json:"application"`
	Snapshot    string    `json:"snapshot"`
	CreatedAt   time.Time `json:"createdAt"`
	Namespace   struct {
		Labels      map[string]string `json:"labels,omitempty"`
		Annotations map[string]string `json:"annotations,omitempty"`
	} `json:"namespace"`
	Volumes   []ManifestVolume `json:"volumes"`
	Resources int              `json:"resources"`
}

// ManifestVolume is a volume of a backup.
type ManifestVolume struct {
	Claim     string `json:"claim"`
	SizeBytes int64  `json:"sizeBytes"`
}

// Exporter copies finished backups to storage locations. It implements snapshot.ExportDriver.
type Exporter struct {
	Locations *location.Store
	Keys      location.KeyReader
	Passwords *Passwords
	Mover     *Mover
	// Bundles holds the saved resources of the local snapshots.
	Bundles *snapshot.Exporter
	Records *RecordStore
	Client  kubernetes.Interface
	// Namespace is the namespace Restora runs in.
	Namespace string
	Log       *slog.Logger
	Now       func() time.Time
}

// Advance implements snapshot.ExportDriver. The locations are copied one after the other, and within a location
// the saved resources go first, then each volume. Every call does a little and returns, so the controller can stop
// at any point and carry on.
func (e *Exporter) Advance(ctx context.Context, snap *snapshot.Snapshot) (bool, error) {
	for i := range snap.Status.Exports {
		es := &snap.Status.Exports[i]
		if es.Finished() {
			continue
		}
		if err := e.advanceLocation(ctx, snap, es); err != nil {
			return false, err
		}
		if !es.Finished() {
			return false, nil
		}
	}
	return true, nil
}

// Cleanup implements snapshot.ExportDriver.
func (e *Exporter) Cleanup(ctx context.Context, snap *snapshot.Snapshot) error {
	return errors.Join(
		e.Mover.Cleanup(ctx, snap.Spec.Application, snap.Name),
		e.Mover.Cleanup(ctx, e.Namespace, snap.Name),
	)
}

func (e *Exporter) advanceLocation(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus) error {
	if es.Phase != snapshot.PhaseRunning {
		now := metav1.NewTime(e.Now())
		es.Phase, es.StartedAt = snapshot.PhaseRunning, &now
	}

	profile, err := e.Locations.Get(ctx, es.Location)
	if errors.Is(err, location.ErrNotFound) {
		return e.fail(ctx, snap, es, CodeLocationMissing, "The storage location does not exist any more.", "Choose another location in the policy.")
	}
	if err != nil {
		return err
	}
	es.LocationName = profile.Spec.Name

	target, failure, err := e.target(ctx, profile)
	if err != nil {
		return err
	}
	if failure != nil {
		return e.fail(ctx, snap, es, failure.Code, failure.Message, failure.Hint)
	}

	if len(es.Volumes) == 0 {
		for _, v := range snap.Status.Volumes {
			es.Volumes = append(es.Volumes, snapshot.ExportVolumeStatus{Claim: v.Claim, Phase: snapshot.PhasePending, SizeBytes: v.SizeBytes})
		}
	}

	if es.ResourcesSnapshot == "" {
		return e.advanceResources(ctx, snap, es, target)
	}
	for i := range es.Volumes {
		ev := &es.Volumes[i]
		if ev.Phase == snapshot.PhaseSucceeded {
			continue
		}
		return e.advanceVolume(ctx, snap, es, ev, i, target)
	}
	return e.complete(ctx, snap, es)
}

// target collects what is needed to open the repository: the profile, the keys and the repository key.
func (e *Exporter) target(ctx context.Context, p *location.LocationProfile) (Target, *snapshot.Failure, error) {
	return BuildTarget(ctx, e.Keys, e.Passwords, p, true)
}

// BuildTarget collects what is needed to open the repository of a location: the profile, the keys and the key of the
// repository. A missing key is a Failure the user can fix, any other problem is an error to retry.
func BuildTarget(ctx context.Context, keys location.KeyReader, passwords *Passwords, p *location.LocationProfile, createKey bool) (Target, *snapshot.Failure, error) {
	t := Target{Location: p.Name, Type: p.Spec.Type, S3: p.Spec.S3, NFS: p.Spec.NFS}
	if p.Spec.Type == location.TypeS3 && p.Spec.S3 != nil {
		access, secret, err := keys.Credentials(ctx, p.Spec.S3.CredentialsSecret)
		if errors.Is(err, location.ErrCredentialsMissing) {
			return t, &snapshot.Failure{Code: location.CodeCredentialsMissing, Message: err.Error(),
				Hint: "Edit the storage location and enter the access key and the secret key again."}, nil
		}
		if err != nil {
			return t, nil, err
		}
		t.AccessKey, t.SecretKey = access, secret
	}
	get := passwords.Existing
	if createKey {
		get = passwords.Get
	}
	password, err := get(ctx, p.Name)
	if errors.Is(err, ErrNoKey) {
		return t, &snapshot.Failure{Code: CodeKeyMissing, Message: err.Error(),
			Hint: "Restore the secret " + PasswordSecretName(p.Name) + " from a copy of the key. Without it the stored backups cannot be read."}, nil
	}
	if err != nil {
		return t, nil, err
	}
	t.Password = password
	return t, nil, nil
}

func locationShort(location string) string {
	return location[strings.LastIndex(location, "-")+1:]
}

func (e *Exporter) baseName(snap *snapshot.Snapshot, es *snapshot.ExportStatus) string {
	return "restora-mover-" + snap.Name + "-" + locationShort(es.Location)
}

func tags(snap *snapshot.Snapshot, kind, volume string) map[string]string {
	t := map[string]string{
		"restora-snapshot":    snap.Name,
		"restora-application": snap.Spec.Application,
		"restora-kind":        kind,
	}
	if volume != "" {
		t["restora-volume"] = volume
	}
	return t
}

// advanceResources stores the saved resources of the backup in the repository.
func (e *Exporter) advanceResources(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus, target Target) error {
	job := Job{
		Name: e.baseName(snap, es) + "-r", Namespace: e.Namespace, Work: snap.Name, Kind: "backup", Target: target,
		Host: snap.Spec.Application, Tags: tags(snap, "resources", ""), Description: snap.Spec.Application + " resources",
	}
	out, err := e.Mover.Status(ctx, job.Namespace, job.Name)
	if err != nil {
		return err
	}
	switch out.State {
	case StateMissing:
		bundle, err := e.bundleFiles(ctx, snap)
		if err != nil {
			var f *snapshot.Failure
			if errors.As(err, &f) {
				return e.fail(ctx, snap, es, f.Code, f.Message, f.Hint)
			}
			return err
		}
		job.Bundle = bundle
		return e.Mover.Start(ctx, job)
	case StateSucceeded:
		es.ResourcesSnapshot = out.Report.SnapshotID
		es.AddedBytes += out.Report.AddedBytes
		e.Log.Info("resources stored in the repository", "snapshot", snap.Name, "location", es.Location, "added", out.Report.AddedBytes)
		return e.Mover.Remove(ctx, job)
	case StateFailed:
		_ = e.Mover.Remove(ctx, job)
		return e.failReport(ctx, snap, es, out.Report)
	default:
		return e.checkStarting(ctx, snap, es, job, nil, out)
	}
}

// bundleFiles reads the saved resources and writes the manifest for them.
func (e *Exporter) bundleFiles(ctx context.Context, snap *snapshot.Snapshot) (map[string][]byte, error) {
	data, err := os.ReadFile(filepath.Join(e.Bundles.Path(snap.Name), bundleFile))
	if err != nil {
		return nil, &snapshot.Failure{Code: CodeBundleMissing, Message: "The saved resources of this backup cannot be read: " + err.Error(),
			Hint: "The data volume of the Restora controller has lost the file. This backup cannot be copied."}
	}
	if len(data) > maxBundleBytes {
		return nil, &snapshot.Failure{Code: CodeBundleTooLarge, Message: fmt.Sprintf("The saved resources are %d KiB, more than the %d KiB that can be copied.", len(data)/1024, maxBundleBytes/1024),
			Hint: "Reduce the number of ConfigMaps, Secrets and other resources of the application."}
	}

	manifest := Manifest{Version: 1, Application: snap.Spec.Application, Snapshot: snap.Name, CreatedAt: snap.CreationTimestamp.UTC()}
	if snap.Status.Resources != nil {
		manifest.Resources = snap.Status.Resources.Count
	}
	for _, v := range snap.Status.Volumes {
		manifest.Volumes = append(manifest.Volumes, ManifestVolume{Claim: v.Claim, SizeBytes: v.SizeBytes})
	}
	if ns, err := e.Client.CoreV1().Namespaces().Get(ctx, snap.Spec.Application, metav1.GetOptions{}); err == nil {
		manifest.Namespace.Labels = withoutRuntimeLabels(ns.Labels)
		manifest.Namespace.Annotations = withoutRuntimeAnnotations(ns.Annotations)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}
	return map[string][]byte{bundleFile: data, manifestFile: encoded}, nil
}

func withoutRuntimeLabels(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		if k != "kubernetes.io/metadata.name" {
			out[k] = v
		}
	}
	return out
}

func withoutRuntimeAnnotations(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		if !strings.HasPrefix(k, "kubectl.kubernetes.io/") && !strings.HasPrefix(k, "cattle.io/") {
			out[k] = v
		}
	}
	return out
}

// advanceVolume copies one volume: a temporary volume is made from its VolumeSnapshot, a data mover reads it and
// stores it in the repository.
func (e *Exporter) advanceVolume(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus, ev *snapshot.ExportVolumeStatus, index int, target Target) error {
	job := Job{
		Name: e.baseName(snap, es) + "-v" + strconv.Itoa(index), Namespace: snap.Spec.Application, Work: snap.Name,
		Kind: "backup", Target: target, Host: snap.Spec.Application, Claim: ev.Claim, ReadOnly: true,
		Tags: tags(snap, "volume", ev.Claim), Description: snap.Spec.Application + "/" + ev.Claim,
	}

	if ev.Phase != snapshot.PhaseRunning {
		if err := e.startVolume(ctx, snap, job, ev); err != nil {
			var f *snapshot.Failure
			if errors.As(err, &f) {
				ev.Phase, ev.Error = snapshot.PhaseFailed, f.Message
				return e.fail(ctx, snap, es, f.Code, f.Message, f.Hint)
			}
			return err
		}
		now := metav1.NewTime(e.Now())
		ev.Phase, ev.Pod, ev.StartedAt = snapshot.PhaseRunning, job.Name, &now
		return nil
	}

	out, err := e.Mover.Status(ctx, job.Namespace, job.Name)
	if err != nil {
		return err
	}
	switch out.State {
	case StateSucceeded:
		ev.KopiaSnapshot, ev.FileCount, ev.AddedBytes = out.Report.SnapshotID, out.Report.FileCount, out.Report.AddedBytes
		if out.Report.SizeBytes > 0 {
			ev.SizeBytes = out.Report.SizeBytes
		}
		es.AddedBytes += out.Report.AddedBytes
		ev.Phase, ev.Pod, ev.StartedAt = snapshot.PhaseSucceeded, "", nil
		e.Log.Info("volume stored in the repository", "snapshot", snap.Name, "location", es.Location, "volume", ev.Claim, "added", out.Report.AddedBytes)
		return e.Mover.Remove(ctx, job)
	case StateFailed:
		_ = e.Mover.Remove(ctx, job)
		ev.Phase, ev.Error = snapshot.PhaseFailed, out.Report.Error
		return e.failReport(ctx, snap, es, out.Report)
	case StateMissing:
		// The pod was deleted from outside. Start over for this volume.
		ev.Phase, ev.Pod = snapshot.PhasePending, ""
		return nil
	default:
		return e.checkStarting(ctx, snap, es, job, ev, out)
	}
}

// startVolume creates the temporary volume and the data mover pod of a volume.
func (e *Exporter) startVolume(ctx context.Context, snap *snapshot.Snapshot, job Job, ev *snapshot.ExportVolumeStatus) error {
	var volumeSnapshot string
	for _, v := range snap.Status.Volumes {
		if v.Claim == ev.Claim {
			volumeSnapshot = v.VolumeSnapshot
		}
	}
	if volumeSnapshot == "" {
		return &snapshot.Failure{Code: CodeFailed, Message: "Volume " + ev.Claim + " has no snapshot.", Hint: "Take a new backup."}
	}

	// The temporary volume copies class and access modes from the claim of the application.
	var class *string
	var modes []corev1.PersistentVolumeAccessMode
	claim, err := e.Client.CoreV1().PersistentVolumeClaims(snap.Spec.Application).Get(ctx, ev.Claim, metav1.GetOptions{})
	switch {
	case err == nil:
		class, modes = claim.Spec.StorageClassName, claim.Spec.AccessModes
	case !apierrors.IsNotFound(err):
		return fmt.Errorf("read volume claim %s: %w", ev.Claim, err)
	}

	size := ev.SizeBytes
	if size == 0 {
		size = 1 << 30
	}
	if err := e.Mover.EnsureClaim(ctx, job, volumeSnapshot, size, class, modes); err != nil {
		return err
	}
	return e.Mover.Start(ctx, job)
}

// checkStarting fails a data mover that does not get going, instead of waiting for it for a day.
func (e *Exporter) checkStarting(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus, job Job, ev *snapshot.ExportVolumeStatus, out Outcome) error {
	if out.Started || out.NotStartedReason == "" {
		return nil
	}
	since := es.StartedAt
	if ev != nil && ev.StartedAt != nil {
		since = ev.StartedAt
	}
	if since == nil || e.Now().Sub(since.Time) < startGrace {
		return nil
	}
	_ = e.Mover.Remove(ctx, job)
	message := "The data mover did not start: " + out.NotStartedReason
	if ev != nil {
		ev.Phase, ev.Error = snapshot.PhaseFailed, message
	}
	return e.fail(ctx, snap, es, CodeMoverNotStarted, message,
		"Check that the cluster can start pods in the namespace of the application and that the image of the Restora controller can be pulled.")
}

// complete records the copy so the application can be restored from the location later.
func (e *Exporter) complete(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus) error {
	spec := RecordSpec{
		Application: snap.Spec.Application, Snapshot: snap.Name, Location: es.Location, LocationName: es.LocationName,
		CreatedAt: snap.CreationTimestamp, Policy: snap.Spec.Policy, ResourcesSnapshot: es.ResourcesSnapshot, AddedBytes: es.AddedBytes,
	}
	if snap.Status.Resources != nil {
		spec.Resources = *snap.Status.Resources
	}
	for _, v := range es.Volumes {
		spec.Volumes = append(spec.Volumes, RecordVolume{Claim: v.Claim, KopiaSnapshot: v.KopiaSnapshot, SizeBytes: v.SizeBytes})
	}
	record := &Record{
		ObjectMeta: metav1.ObjectMeta{
			Name: RecordName(snap.Name, es.Location),
			Labels: map[string]string{
				snapshot.LabelManagedBy: snapshot.ManagedByValue, snapshot.LabelApplication: snap.Spec.Application,
				snapshot.LabelSnapshot: snap.Name, LabelLocation: es.Location,
			},
		},
		Spec: spec,
	}
	if _, err := e.Records.Create(ctx, record); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	finished := metav1.NewTime(e.Now())
	es.Phase, es.FinishedAt, es.ErrorCode, es.Message, es.Hint = snapshot.PhaseSucceeded, &finished, "", "", ""
	e.Log.Info("backup copied to storage location", "snapshot", snap.Name, "location", es.Location, "added", es.AddedBytes)
	return nil
}

func (e *Exporter) failReport(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus, r MoverReport) error {
	code := r.Code
	if code == "" {
		code = CodeFailed
	}
	return e.fail(ctx, snap, es, code, r.Error, hintFor(code))
}

// fail ends the copy to one location. The other locations carry on, and the local backup stays valid.
func (e *Exporter) fail(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus, code, message, hint string) error {
	finished := metav1.NewTime(e.Now())
	es.Phase, es.ErrorCode, es.Message, es.Hint, es.FinishedAt = snapshot.PhaseFailed, code, message, hint, &finished
	for i := range es.Volumes {
		if es.Volumes[i].Phase == snapshot.PhaseRunning {
			es.Volumes[i].Phase, es.Volumes[i].Pod = snapshot.PhaseFailed, ""
		}
	}
	e.Log.Warn("copy to storage location failed", "snapshot", snap.Name, "location", es.Location, "code", code, "message", message)
	return e.removeLocationJobs(ctx, snap, es)
}

// removeLocationJobs removes the pods, secrets and temporary volumes of one location.
func (e *Exporter) removeLocationJobs(ctx context.Context, snap *snapshot.Snapshot, es *snapshot.ExportStatus) error {
	base := e.baseName(snap, es)
	errs := []error{e.Mover.Remove(ctx, Job{Name: base + "-r", Namespace: e.Namespace, Bundle: map[string][]byte{}})}
	for i := range es.Volumes {
		errs = append(errs, e.Mover.Remove(ctx, Job{
			Name: base + "-v" + strconv.Itoa(i), Namespace: snap.Spec.Application, Kind: "backup", Claim: es.Volumes[i].Claim,
		}))
	}
	return errors.Join(errs...)
}

// Codes of a failed copy besides the ones a data mover reports.
const (
	CodeLocationMissing = "location_missing"
	CodeBundleMissing   = "bundle_missing"
	CodeBundleTooLarge  = "bundle_too_large"
	CodeMoverNotStarted = "mover_not_started"
	CodeKeyMissing      = "repository_key_missing"
)

// hintFor gives the English hint for a failure code. The interface shows its own translated text.
func hintFor(code string) string {
	switch code {
	case CodeWrongPassword:
		return "The repository in this location is encrypted with another key than the one this cluster holds."
	case CodeAccessDenied:
		return "The keys of the storage location may not write to it. Test the location and correct the keys."
	case CodeBucketNotFound:
		return "The bucket does not exist. Test the location and correct its settings."
	case CodeEndpointUnreachable:
		return "The data mover cannot reach the storage. Test the location and check the network."
	case CodeNotWritable:
		return "The share is not writable for the data mover. It runs as root, so the export must not squash root."
	default:
		return "See the technical details."
	}
}
