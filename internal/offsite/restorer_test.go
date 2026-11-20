package offsite

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"restora.local/restora/internal/snapshot"
)

// restoreWorld is a restorer in a fake cluster, with an export record that points to a location with a stored key.
type restoreWorld struct {
	*world
	rs   *Restorer
	rec  *Record
	logs string
}

func newRestoreWorld(t *testing.T) *restoreWorld {
	t.Helper()
	w := newWorld(t, location1)
	// The key the export used is in the cluster.
	if _, err := w.ex.Passwords.Get(context.Background(), location1); err != nil {
		t.Fatal(err)
	}
	rw := &restoreWorld{world: w}
	rw.rs = &Restorer{
		Locations: w.ex.Locations, Keys: w.ex.Keys, Passwords: w.ex.Passwords, Mover: w.ex.Mover, Bundles: w.ex.Bundles,
		Client: w.kube, Namespace: testNamespace, Log: discardLog(), Now: func() time.Time { return w.now },
		Logs: func(context.Context, string, string) (string, error) { return rw.logs, nil },
	}
	rw.rec = &Record{
		ObjectMeta: metav1.ObjectMeta{Name: "x-1"},
		Spec: RecordSpec{
			Application: testApp, Snapshot: "s-1", Location: location1, LocationName: "Storage", ResourcesSnapshot: "res-1",
			Volumes: []RecordVolume{{Claim: "data", KopiaSnapshot: "vol-1", SizeBytes: 100}, {Claim: "logs", KopiaSnapshot: "vol-2", SizeBytes: 5}},
		},
	}
	rw.logs = bundleBegin + " resources.json.gz\n" + base64.StdEncoding.EncodeToString([]byte("gz-bytes")) + "\n" + bundleEnd + "\n" +
		bundleBegin + " manifest.json\n" + base64.StdEncoding.EncodeToString([]byte(`{"version":1,"application":"demo","namespace":{"labels":{"team":"a"}}}`)) + "\n" + bundleEnd + "\n"
	return rw
}

func TestPrepareFetchesTheSavedResources(t *testing.T) {
	w := newRestoreWorld(t)
	ctx := context.Background()

	if done, err := w.rs.Prepare(ctx, "r-1", w.rec); err != nil || done {
		t.Fatalf("first pass: done %v, err %v; it only starts the data mover", done, err)
	}
	podName := "restora-mover-r-1-b"
	pod, err := w.kube.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("pod: %v", err)
	}
	if got := pod.Spec.Containers[0].Args; len(got) != 2 || got[1] != "fetch-bundle" {
		t.Errorf("args = %v", got)
	}
	var snapshotID string
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == envMoverSnapshot {
			snapshotID = e.Value
		}
	}
	if snapshotID != "res-1" {
		t.Errorf("the mover must fetch the resources of the export, got %q", snapshotID)
	}

	if done, err := w.rs.Prepare(ctx, "r-1", w.rec); err != nil || done {
		t.Fatalf("while it runs: done %v, err %v", done, err)
	}
	w.finish(testNamespace, podName, 0, MoverReport{SnapshotID: "res-1"})

	if done, err := w.rs.Prepare(ctx, "r-1", w.rec); err != nil || !done {
		t.Fatalf("after the mover: done %v, err %v", done, err)
	}
	dir := w.ex.Bundles.Path(StageName("r-1"))
	if data, _ := os.ReadFile(filepath.Join(dir, "resources.json.gz")); string(data) != "gz-bytes" {
		t.Errorf("staged resources = %q", data)
	}
	manifest, err := w.rs.Manifest("r-1")
	if err != nil || manifest.Application != testApp || manifest.Namespace.Labels["team"] != "a" {
		t.Errorf("manifest = %+v, %v", manifest, err)
	}
	if got := w.pods(testNamespace); len(got) != 0 {
		t.Errorf("the pod must be removed: %v", got)
	}

	// Once staged, nothing is fetched again.
	if done, err := w.rs.Prepare(ctx, "r-1", w.rec); err != nil || !done {
		t.Errorf("staged: done %v, err %v", done, err)
	}
}

func TestPrepareReportsWhatWentWrong(t *testing.T) {
	ctx := context.Background()

	t.Run("the key is not in this cluster", func(t *testing.T) {
		w := newRestoreWorld(t)
		if err := w.kube.CoreV1().Secrets(testNamespace).Delete(ctx, PasswordSecretName(location1), metav1.DeleteOptions{}); err != nil {
			t.Fatal(err)
		}
		_, err := w.rs.Prepare(ctx, "r-1", w.rec)
		var f *snapshot.Failure
		if !errors.As(err, &f) || f.Code != CodeKeyMissing || f.Hint == "" {
			t.Errorf("error = %v, want the missing key", err)
		}
		if _, err := w.ex.Passwords.Existing(ctx, location1); !errors.Is(err, ErrNoKey) {
			t.Errorf("a restore must never make a new key: %v", err)
		}
	})

	t.Run("the location is gone", func(t *testing.T) {
		w := newRestoreWorld(t)
		if err := w.ex.Locations.Delete(ctx, location1); err != nil {
			t.Fatal(err)
		}
		_, err := w.rs.Prepare(ctx, "r-1", w.rec)
		var f *snapshot.Failure
		if !errors.As(err, &f) || f.Code != CodeLocationMissing {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the mover fails", func(t *testing.T) {
		w := newRestoreWorld(t)
		w.rs.Prepare(ctx, "r-1", w.rec)
		w.finish(testNamespace, "restora-mover-r-1-b", 1, MoverReport{Code: CodeWrongPassword, Error: "invalid repository password"})
		_, err := w.rs.Prepare(ctx, "r-1", w.rec)
		var f *snapshot.Failure
		if !errors.As(err, &f) || f.Code != CodeWrongPassword {
			t.Errorf("error = %v", err)
		}
		if got := w.pods(testNamespace); len(got) != 0 {
			t.Errorf("pods = %v", got)
		}
	})

	t.Run("the output has no files", func(t *testing.T) {
		w := newRestoreWorld(t)
		w.logs = "just a log line\n"
		w.rs.Prepare(ctx, "r-1", w.rec)
		w.finish(testNamespace, "restora-mover-r-1-b", 0, MoverReport{})
		_, err := w.rs.Prepare(ctx, "r-1", w.rec)
		var f *snapshot.Failure
		if !errors.As(err, &f) || f.Code != CodeBundleMissing {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the mover never starts", func(t *testing.T) {
		w := newRestoreWorld(t)
		w.rs.Prepare(ctx, "r-1", w.rec)
		pod, _ := w.kube.CoreV1().Pods(testNamespace).Get(ctx, "restora-mover-r-1-b", metav1.GetOptions{})
		pod.CreationTimestamp = metav1.NewTime(testNow.Add(-10 * time.Minute))
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}
		w.kube.CoreV1().Pods(testNamespace).Update(ctx, pod, metav1.UpdateOptions{})
		w.kube.CoreV1().Pods(testNamespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})

		_, err := w.rs.Prepare(ctx, "r-1", w.rec)
		var f *snapshot.Failure
		if !errors.As(err, &f) || f.Code != CodeMoverNotStarted {
			t.Errorf("error = %v", err)
		}
	})
}

func TestRestoreVolumeRunsAMoverIntoTheClaim(t *testing.T) {
	w := newRestoreWorld(t)
	ctx := context.Background()

	if done, err := w.rs.RestoreVolume(ctx, "r-1", w.rec, 1); err != nil || done {
		t.Fatalf("first pass: done %v, err %v", done, err)
	}
	name := "restora-mover-r-1-v1"
	pod, err := w.kube.CoreV1().Pods(testApp).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("pod: %v", err)
	}
	var volume corev1.Volume
	for _, v := range pod.Spec.Volumes {
		if v.Name == "data" {
			volume = v
		}
	}
	if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != "logs" || volume.PersistentVolumeClaim.ReadOnly {
		t.Errorf("the mover must mount the claim of the application for writing: %+v", volume)
	}
	var snapshotID string
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == envMoverSnapshot {
			snapshotID = e.Value
		}
	}
	if snapshotID != "vol-2" || pod.Spec.Containers[0].Args[1] != "restore" {
		t.Errorf("snapshot id = %q, args = %v", snapshotID, pod.Spec.Containers[0].Args)
	}

	if done, err := w.rs.RestoreVolume(ctx, "r-1", w.rec, 1); err != nil || done {
		t.Fatalf("while it runs: done %v, err %v", done, err)
	}
	w.finish(testApp, name, 0, MoverReport{SnapshotID: "vol-2"})
	if done, err := w.rs.RestoreVolume(ctx, "r-1", w.rec, 1); err != nil || !done {
		t.Fatalf("after the mover: done %v, err %v", done, err)
	}
	if got := w.pods(testApp); len(got) != 0 {
		t.Errorf("pods left: %v", got)
	}
}

func TestRestoreVolumeFailsWithTheReasonOfTheMover(t *testing.T) {
	w := newRestoreWorld(t)
	ctx := context.Background()
	w.rs.RestoreVolume(ctx, "r-1", w.rec, 0)
	w.finish(testApp, "restora-mover-r-1-v0", 1, MoverReport{Code: CodeEndpointUnreachable, Error: "dial tcp: i/o timeout"})

	_, err := w.rs.RestoreVolume(ctx, "r-1", w.rec, 0)
	var f *snapshot.Failure
	if !errors.As(err, &f) || f.Code != CodeEndpointUnreachable || f.Hint == "" {
		t.Errorf("error = %v", err)
	}
}

func TestRestoreCleanupRemovesMoversAndTheStagedFiles(t *testing.T) {
	w := newRestoreWorld(t)
	ctx := context.Background()
	w.rs.Prepare(ctx, "r-1", w.rec)
	w.rs.RestoreVolume(ctx, "r-1", w.rec, 0)
	dir := w.ex.Bundles.Path(StageName("r-1"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "resources.json.gz"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := w.rs.Cleanup(ctx, "r-1", testApp); err != nil {
		t.Fatal(err)
	}
	if len(w.pods(testApp)) != 0 || len(w.pods(testNamespace)) != 0 {
		t.Errorf("pods left: %v %v", w.pods(testApp), w.pods(testNamespace))
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("staged files left: %v", err)
	}
}
