package offsite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"restora.local/restora/internal/location"
	"restora.local/restora/internal/snapshot"
)

// StageName is the name under which the saved resources of a restore are kept while it runs. It lives below the
// same directory as the local backups, so the restore reads both the same way.
func StageName(requestName string) string {
	return "restore-" + requestName
}

// Restorer brings backups back from storage locations: it fetches the saved resources, and runs a data mover that
// fills a volume claim from the repository.
type Restorer struct {
	Locations *location.Store
	Keys      location.KeyReader
	Passwords *Passwords
	Mover     *Mover
	// Bundles holds the staging directory of the saved resources.
	Bundles *snapshot.Exporter
	Client  kubernetes.Interface
	// Namespace is the namespace Restora runs in.
	Namespace string
	// Logs reads the log of a pod. It defaults to the Kubernetes API.
	Logs func(ctx context.Context, namespace, pod string) (string, error)
	Log  *slog.Logger
	Now  func() time.Time
}

func (r *Restorer) target(ctx context.Context, rec *Record) (Target, error) {
	profile, err := r.Locations.Get(ctx, rec.Spec.Location)
	if errors.Is(err, location.ErrNotFound) {
		return Target{}, &snapshot.Failure{Code: CodeLocationMissing,
			Message: "The storage location " + rec.Spec.LocationName + " does not exist any more.",
			Hint:    "Add the storage location again and restore from a new backup, or restore the profile with the same name."}
	}
	if err != nil {
		return Target{}, err
	}
	target, failure, err := BuildTarget(ctx, r.Keys, r.Passwords, profile, false)
	if err != nil {
		return Target{}, err
	}
	if failure != nil {
		return Target{}, failure
	}
	return target, nil
}

func (r *Restorer) readLogs(ctx context.Context, namespace, pod string) (string, error) {
	if r.Logs != nil {
		return r.Logs(ctx, namespace, pod)
	}
	stream, err := r.Client.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	var out []byte
	buf := make([]byte, 64<<10)
	for {
		n, readErr := stream.Read(buf)
		out = append(out, buf[:n]...)
		if readErr != nil {
			break
		}
		if len(out) > 8<<20 {
			return "", errors.New("the log of the data mover is too large")
		}
	}
	return string(out), nil
}

// Prepare fetches the saved resources and the manifest of an export from the repository into the staging
// directory. It returns false while the data mover is still working.
func (r *Restorer) Prepare(ctx context.Context, requestName string, rec *Record) (bool, error) {
	dir := r.Bundles.Path(StageName(requestName))
	if _, err := os.Stat(filepath.Join(dir, bundleFile)); err == nil {
		return true, nil
	}
	target, err := r.target(ctx, rec)
	if err != nil {
		return false, err
	}
	job := Job{
		Name: "restora-mover-" + requestName + "-b", Namespace: r.Namespace, Work: requestName, Kind: "fetch-bundle",
		Target: target, Host: rec.Spec.Application, SnapshotID: rec.Spec.ResourcesSnapshot,
	}
	out, err := r.Mover.Status(ctx, job.Namespace, job.Name)
	if err != nil {
		return false, err
	}
	switch out.State {
	case StateMissing:
		return false, r.Mover.Start(ctx, job)
	case StateSucceeded:
		logs, err := r.readLogs(ctx, job.Namespace, job.Name)
		if err != nil {
			return false, fmt.Errorf("read the saved resources from the data mover: %w", err)
		}
		files, err := ParseFiles(logs)
		if err != nil || files[bundleFile] == nil {
			_ = r.Mover.Remove(ctx, job)
			return false, &snapshot.Failure{Code: CodeBundleMissing, Message: "The saved resources could not be read from the repository.",
				Hint: "The storage location does not hold the saved resources of this backup."}
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return false, err
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
				return false, err
			}
		}
		return true, r.Mover.Remove(ctx, job)
	case StateFailed:
		_ = r.Mover.Remove(ctx, job)
		return false, r.failure(out.Report)
	default:
		return false, r.checkStart(ctx, job, out)
	}
}

// Manifest reads the manifest of the export that Prepare fetched.
func (r *Restorer) Manifest(requestName string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(r.Bundles.Path(StageName(requestName)), manifestFile))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

// RestoreVolume fills the volume claim of the application with the volume of an export. The claim must exist and be
// empty. It returns false while the data mover is still working.
func (r *Restorer) RestoreVolume(ctx context.Context, requestName string, rec *Record, index int) (bool, error) {
	vol := rec.Spec.Volumes[index]
	target, err := r.target(ctx, rec)
	if err != nil {
		return false, err
	}
	job := Job{
		Name: "restora-mover-" + requestName + "-v" + strconv.Itoa(index), Namespace: rec.Spec.Application, Work: requestName,
		Kind: "restore", Target: target, Host: rec.Spec.Application, Claim: vol.Claim, SnapshotID: vol.KopiaSnapshot,
	}
	out, err := r.Mover.Status(ctx, job.Namespace, job.Name)
	if err != nil {
		return false, err
	}
	switch out.State {
	case StateMissing:
		return false, r.Mover.Start(ctx, job)
	case StateSucceeded:
		return true, r.Mover.Remove(ctx, job)
	case StateFailed:
		_ = r.Mover.Remove(ctx, job)
		return false, r.failure(out.Report)
	default:
		return false, r.checkStart(ctx, job, out)
	}
}

// Cleanup removes the data mover pods of a restore and the staging directory.
func (r *Restorer) Cleanup(ctx context.Context, requestName, application string) error {
	return errors.Join(
		r.Mover.Cleanup(ctx, application, requestName),
		r.Mover.Cleanup(ctx, r.Namespace, requestName),
		r.Bundles.Delete(StageName(requestName)),
	)
}

func (r *Restorer) failure(report MoverReport) error {
	code := report.Code
	if code == "" {
		code = CodeFailed
	}
	return &snapshot.Failure{Code: code, Message: report.Error, Hint: hintFor(code)}
}

// checkStart fails a data mover that does not get going.
func (r *Restorer) checkStart(ctx context.Context, job Job, out Outcome) error {
	if out.Started || out.NotStartedReason == "" || out.CreatedAt.IsZero() || r.Now().Sub(out.CreatedAt) < startGrace {
		return nil
	}
	_ = r.Mover.Remove(ctx, job)
	return &snapshot.Failure{Code: CodeMoverNotStarted, Message: "The data mover did not start: " + out.NotStartedReason,
		Hint: "Check that the cluster can start pods in the namespace of the application and that the image of the Restora controller can be pulled."}
}
