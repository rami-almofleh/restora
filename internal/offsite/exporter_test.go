package offsite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"restora.local/restora/internal/location"
	"restora.local/restora/internal/snapshot"
)

const (
	testNamespace = "restora-system"
	testApp       = "demo"
)

var testNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

type fixedKeys struct{ err error }

func (k fixedKeys) Credentials(context.Context, string) (string, string, error) {
	if k.err != nil {
		return "", "", k.err
	}
	return "AKIATEST", "very-secret-value", nil
}

// world is an exporter with a fake cluster and one backup that is ready to be copied.
type world struct {
	t       *testing.T
	ex      *Exporter
	kube    *kubefake.Clientset
	records *RecordStore
	snap    *snapshot.Snapshot
	now     time.Time
}

func newWorld(t *testing.T, locations ...string) *world {
	t.Helper()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		location.GVR: "LocationProfileList", GVR: "ExportList",
	})
	kube := kubefake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: testApp, Labels: map[string]string{"team": "a", "kubernetes.io/metadata.name": testApp},
	}}, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: testApp},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: new("csi-hostpath-sc"), AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
	})
	store := location.NewStore(dyn, testNamespace)
	for _, id := range locations {
		if _, err := store.Create(context.Background(), &location.LocationProfile{
			ObjectMeta: metav1.ObjectMeta{Name: id},
			Spec: location.Spec{Name: "Storage " + id, Type: location.TypeS3, S3: &location.S3Spec{
				Endpoint: "http://minio.minio:9000", Bucket: "restora", CredentialsSecret: "restora-location-" + id,
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	bundles := snapshot.NewExporter(dyn, dir)
	snapName := "s-20261120-110000-abcd"
	if err := os.MkdirAll(filepath.Join(dir, snapName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, snapName, "resources.json.gz"), []byte("bundle-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &world{t: t, kube: kube, records: NewRecordStore(dyn, testNamespace), now: testNow}
	w.ex = &Exporter{
		Locations: store, Keys: fixedKeys{}, Passwords: &Passwords{Client: kube, Namespace: testNamespace},
		Mover:   &Mover{Client: kube, Image: "restora-controller:dev", PullPolicy: corev1.PullIfNotPresent},
		Bundles: bundles, Records: w.records, Client: kube, Namespace: testNamespace, Log: discardLog(),
		Now: func() time.Time { return w.now },
	}
	w.snap = &snapshot.Snapshot{
		ObjectMeta: metav1.ObjectMeta{Name: snapName, CreationTimestamp: metav1.NewTime(testNow.Add(-time.Hour))},
		Spec:       snapshot.Spec{Application: testApp, ExportTo: locations, Policy: &snapshot.PolicyRef{ID: "p-1", Name: "Nightly"}},
		Status: snapshot.Status{
			Phase:     snapshot.PhaseRunning,
			Resources: &snapshot.ResourceStats{Count: 4, Bytes: 12},
			Volumes:   []snapshot.VolumeStatus{{Claim: "data", VolumeSnapshot: snapName + "-0", Ready: true, SizeBytes: 1 << 30}},
		},
	}
	for _, id := range locations {
		w.snap.Status.Exports = append(w.snap.Status.Exports, snapshot.ExportStatus{Location: id, Phase: snapshot.PhasePending})
	}
	return w
}

func (w *world) advance() bool {
	w.t.Helper()
	done, err := w.ex.Advance(context.Background(), w.snap)
	if err != nil {
		w.t.Fatalf("Advance: %v", err)
	}
	return done
}

// finish ends a pod the way a data mover does: with a termination message.
func (w *world) finish(namespace, name string, exitCode int32, report MoverReport) {
	w.t.Helper()
	pod, err := w.kube.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		w.t.Fatalf("pod %s/%s: %v", namespace, name, err)
	}
	message, _ := json.Marshal(report)
	pod.Status.Phase = corev1.PodSucceeded
	if exitCode != 0 {
		pod.Status.Phase = corev1.PodFailed
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "mover", State: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{ExitCode: exitCode, Message: string(message), Reason: "Completed"},
	}}}
	if _, err := w.kube.CoreV1().Pods(namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) pods(namespace string) []string {
	list, _ := w.kube.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	var names []string
	for _, p := range list.Items {
		names = append(names, p.Name)
	}
	return names
}

const location1 = "l-20261120-100000-aaaa"
const location2 = "l-20261120-100000-bbbb"

func TestCopyOfAFullBackup(t *testing.T) {
	w := newWorld(t, location1)
	base := "restora-mover-" + w.snap.Name + "-aaaa"

	if w.advance() {
		t.Fatal("the first pass must only start the data mover for the resources")
	}
	if got := w.pods(testNamespace); len(got) != 1 || got[0] != base+"-r" {
		t.Fatalf("pods = %v", got)
	}
	secrets, _ := w.kube.CoreV1().Secrets(testNamespace).List(context.Background(), metav1.ListOptions{})
	var bundle *corev1.Secret
	for i := range secrets.Items {
		if secrets.Items[i].Name == base+"-r-bundle" {
			bundle = &secrets.Items[i]
		}
	}
	if bundle == nil || string(bundle.Data["resources.json.gz"]) != "bundle-bytes" {
		t.Fatalf("the saved resources must be handed to the mover as a secret: %+v", bundle)
	}
	var manifest Manifest
	if err := json.Unmarshal(bundle.Data["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Application != testApp || manifest.Snapshot != w.snap.Name || len(manifest.Volumes) != 1 || manifest.Namespace.Labels["team"] != "a" {
		t.Errorf("manifest = %+v", manifest)
	}
	if _, ok := manifest.Namespace.Labels["kubernetes.io/metadata.name"]; ok {
		t.Error("the namespace label Kubernetes sets itself must not be saved")
	}

	w.finish(testNamespace, base+"-r", 0, MoverReport{SnapshotID: "res-1", SizeBytes: 12, AddedBytes: 2048})
	if w.advance() {
		t.Fatal("volumes are still to come")
	}
	if es := w.snap.Status.Exports[0]; es.ResourcesSnapshot != "res-1" || es.AddedBytes != 2048 {
		t.Fatalf("export = %+v", es)
	}
	if got := w.pods(testNamespace); len(got) != 0 {
		t.Errorf("the pod of the resources must be removed: %v", got)
	}

	w.advance() // starts the volume
	if got := w.pods(testApp); len(got) != 1 || got[0] != base+"-v0" {
		t.Fatalf("pods in the application = %v", got)
	}
	claim, err := w.kube.CoreV1().PersistentVolumeClaims(testApp).Get(context.Background(), base+"-v0", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("temporary volume: %v", err)
	}
	if ds := claim.Spec.DataSource; ds == nil || ds.Kind != "VolumeSnapshot" || ds.Name != w.snap.Name+"-0" || *claim.Spec.StorageClassName != "csi-hostpath-sc" {
		t.Errorf("claim = %+v", claim.Spec)
	}
	if got := w.snap.Status.Exports[0].Volumes[0]; got.Phase != snapshot.PhaseRunning || got.Pod != base+"-v0" {
		t.Errorf("volume = %+v", got)
	}

	w.finish(testApp, base+"-v0", 0, MoverReport{SnapshotID: "vol-1", SizeBytes: 3000006, FileCount: 2, AddedBytes: 3000000})
	if w.advance() {
		t.Fatal("recording the copy is the last pass")
	}
	if !w.advance() {
		t.Fatal("the copy must be done now")
	}

	es := w.snap.Status.Exports[0]
	if es.Phase != snapshot.PhaseSucceeded || es.AddedBytes != 3002048 || es.Volumes[0].KopiaSnapshot != "vol-1" || es.LocationName != "Storage "+location1 {
		t.Errorf("export = %+v", es)
	}
	if got := w.pods(testApp); len(got) != 0 {
		t.Errorf("pods left in the application: %v", got)
	}
	if list, _ := w.kube.CoreV1().PersistentVolumeClaims(testApp).List(context.Background(), metav1.ListOptions{}); len(list.Items) != 1 {
		t.Errorf("only the claim of the application may be left, got %d", len(list.Items))
	}

	record, err := w.records.Get(context.Background(), RecordName(w.snap.Name, location1))
	if err != nil {
		t.Fatalf("export record: %v", err)
	}
	spec := record.Spec
	if spec.Application != testApp || spec.ResourcesSnapshot != "res-1" || len(spec.Volumes) != 1 || spec.Volumes[0].KopiaSnapshot != "vol-1" ||
		spec.Policy == nil || spec.Policy.Name != "Nightly" || spec.Resources.Count != 4 {
		t.Errorf("record = %+v", spec)
	}
}

func TestMoversGetTheRepositoryAndTheKeysOnlyThroughASecret(t *testing.T) {
	w := newWorld(t, location1)
	w.advance()

	pod, _ := w.kube.CoreV1().Pods(testNamespace).Get(context.Background(), "restora-mover-"+w.snap.Name+"-aaaa-r", metav1.GetOptions{})
	c := pod.Spec.Containers[0]
	for _, e := range c.Env {
		if strings.Contains(e.Value, "very-secret-value") || strings.Contains(e.Value, "AKIATEST") {
			t.Errorf("env %s must not hold a key: it would show in the pod", e.Name)
		}
	}
	if len(c.EnvFrom) != 1 || c.EnvFrom[0].SecretRef == nil {
		t.Fatalf("envFrom = %+v", c.EnvFrom)
	}
	secret, err := w.kube.CoreV1().Secrets(testNamespace).Get(context.Background(), c.EnvFrom[0].SecretRef.Name, metav1.GetOptions{})
	if err != nil || secret.StringData["RESTORA_REPO_SECRET_KEY"] != "very-secret-value" || secret.StringData["RESTORA_REPO_PASSWORD"] == "" {
		t.Errorf("secret = %+v, %v", secret, err)
	}
}

func TestTheRepositoryKeyIsMadeOnceAndKept(t *testing.T) {
	w := newWorld(t, location1)
	w.advance()
	first, err := w.ex.Passwords.Get(context.Background(), location1)
	if err != nil || len(first) < 32 {
		t.Fatalf("password = %q, %v", first, err)
	}
	second, _ := w.ex.Passwords.Get(context.Background(), location1)
	if second != first {
		t.Error("the key must never change: stored backups could not be read any more")
	}
	other, _ := w.ex.Passwords.Get(context.Background(), location2)
	if other == first {
		t.Error("every location has its own key")
	}
}

func TestACopyFailsWithTheReasonTheMoverGives(t *testing.T) {
	w := newWorld(t, location1)
	base := "restora-mover-" + w.snap.Name + "-aaaa"
	w.advance()
	w.finish(testNamespace, base+"-r", 1, MoverReport{Code: CodeWrongPassword, Error: "invalid repository password"})

	if !w.advance() {
		t.Fatal("a failed copy is finished")
	}
	es := w.snap.Status.Exports[0]
	if es.Phase != snapshot.PhaseFailed || es.ErrorCode != CodeWrongPassword || es.Message == "" || es.Hint == "" || es.FinishedAt == nil {
		t.Errorf("export = %+v", es)
	}
	if got := w.pods(testNamespace); len(got) != 0 {
		t.Errorf("the failed mover must be removed: %v", got)
	}
	if _, err := w.records.Get(context.Background(), RecordName(w.snap.Name, location1)); !apierrors.IsNotFound(unwrap(err)) && err == nil {
		t.Error("a failed copy must not leave a record")
	}
}

func unwrap(err error) error { return err }

func TestAFailedVolumeFailsTheCopyAndCleansUp(t *testing.T) {
	w := newWorld(t, location1)
	base := "restora-mover-" + w.snap.Name + "-aaaa"
	w.advance()
	w.finish(testNamespace, base+"-r", 0, MoverReport{SnapshotID: "res-1"})
	w.advance()
	w.advance() // volume started
	w.finish(testApp, base+"-v0", 1, MoverReport{Code: CodeAccessDenied, Error: "AccessDenied"})

	if !w.advance() {
		t.Fatal("a failed copy is finished")
	}
	es := w.snap.Status.Exports[0]
	if es.Phase != snapshot.PhaseFailed || es.ErrorCode != CodeAccessDenied || es.Volumes[0].Phase != snapshot.PhaseFailed || es.Volumes[0].Error == "" {
		t.Errorf("export = %+v", es)
	}
	if got := w.pods(testApp); len(got) != 0 {
		t.Errorf("pods left: %v", got)
	}
	if list, _ := w.kube.CoreV1().PersistentVolumeClaims(testApp).List(context.Background(), metav1.ListOptions{}); len(list.Items) != 1 {
		t.Errorf("the temporary volume must go: %d claims", len(list.Items))
	}
}

func TestOneFailingLocationDoesNotStopTheNext(t *testing.T) {
	w := newWorld(t, location1, location2)
	// The first location is gone.
	if err := w.ex.Locations.Delete(context.Background(), location1); err != nil {
		t.Fatal(err)
	}

	if w.advance() {
		t.Fatal("the second location still has to be copied")
	}
	first, second := w.snap.Status.Exports[0], w.snap.Status.Exports[1]
	if first.Phase != snapshot.PhaseFailed || first.ErrorCode != CodeLocationMissing {
		t.Errorf("first = %+v", first)
	}
	if second.Phase != snapshot.PhaseRunning {
		t.Errorf("second = %+v, want it to have started", second)
	}
}

func TestMissingKeysFailTheCopyWithoutStartingAnything(t *testing.T) {
	w := newWorld(t, location1)
	w.ex.Keys = fixedKeys{err: location.ErrCredentialsMissing}

	if !w.advance() {
		t.Fatal("want the copy to end")
	}
	es := w.snap.Status.Exports[0]
	if es.ErrorCode != location.CodeCredentialsMissing {
		t.Errorf("export = %+v", es)
	}
	if got := w.pods(testNamespace); len(got) != 0 {
		t.Errorf("pods = %v", got)
	}
}

func TestAMoverThatNeverStartsFailsTheCopy(t *testing.T) {
	w := newWorld(t, location1)
	base := "restora-mover-" + w.snap.Name + "-aaaa"
	w.advance()

	// The pod waits for an image that cannot be pulled.
	pod, _ := w.kube.CoreV1().Pods(testNamespace).Get(context.Background(), base+"-r", metav1.GetOptions{})
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "mover", State: corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull access denied"},
	}}}
	if _, err := w.kube.CoreV1().Pods(testNamespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	w.now = testNow.Add(time.Minute)
	if w.advance() {
		t.Fatal("one minute is within the grace period")
	}
	w.now = testNow.Add(6 * time.Minute)
	if !w.advance() {
		t.Fatal("after the grace period the copy must fail")
	}
	es := w.snap.Status.Exports[0]
	if es.ErrorCode != CodeMoverNotStarted || !strings.Contains(es.Message, "ImagePullBackOff") {
		t.Errorf("export = %+v", es)
	}
	if got := w.pods(testNamespace); len(got) != 0 {
		t.Errorf("pods = %v", got)
	}
}

func TestAnOversizedBundleIsRefused(t *testing.T) {
	w := newWorld(t, location1)
	big := make([]byte, maxBundleBytes+1)
	if err := os.WriteFile(filepath.Join(w.ex.Bundles.Path(w.snap.Name), "resources.json.gz"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	if !w.advance() {
		t.Fatal("want the copy to end")
	}
	if es := w.snap.Status.Exports[0]; es.ErrorCode != CodeBundleTooLarge {
		t.Errorf("export = %+v", es)
	}
}

func TestAMissingBundleIsReported(t *testing.T) {
	w := newWorld(t, location1)
	if err := os.Remove(filepath.Join(w.ex.Bundles.Path(w.snap.Name), "resources.json.gz")); err != nil {
		t.Fatal(err)
	}
	if !w.advance() {
		t.Fatal("want the copy to end")
	}
	if es := w.snap.Status.Exports[0]; es.ErrorCode != CodeBundleMissing {
		t.Errorf("export = %+v", es)
	}
}

func TestAMoverThatDisappearsIsStartedAgain(t *testing.T) {
	w := newWorld(t, location1)
	base := "restora-mover-" + w.snap.Name + "-aaaa"
	w.advance()
	w.finish(testNamespace, base+"-r", 0, MoverReport{SnapshotID: "res-1"})
	w.advance()
	w.advance() // volume started
	if err := w.kube.CoreV1().Pods(testApp).Delete(context.Background(), base+"-v0", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}

	w.advance() // notices
	w.advance() // starts again
	if got := w.pods(testApp); len(got) != 1 {
		t.Errorf("pods = %v, want the mover to be back", got)
	}
}

func TestCleanupRemovesEverythingOfASnapshot(t *testing.T) {
	w := newWorld(t, location1)
	base := "restora-mover-" + w.snap.Name + "-aaaa"
	w.advance()
	w.finish(testNamespace, base+"-r", 0, MoverReport{SnapshotID: "res-1"})
	w.advance()
	w.advance() // volume started: pod, secret and claim exist in the application

	if err := w.ex.Cleanup(context.Background(), w.snap); err != nil {
		t.Fatal(err)
	}
	if got := w.pods(testApp); len(got) != 0 {
		t.Errorf("pods = %v", got)
	}
	if list, _ := w.kube.CoreV1().PersistentVolumeClaims(testApp).List(context.Background(), metav1.ListOptions{}); len(list.Items) != 1 {
		t.Errorf("claims = %d, only the one of the application may stay", len(list.Items))
	}
	secrets, _ := w.kube.CoreV1().Secrets(testApp).List(context.Background(), metav1.ListOptions{})
	if len(secrets.Items) != 0 {
		t.Errorf("secrets = %d", len(secrets.Items))
	}
}

func TestABackupWithoutVolumesOnlyStoresTheResources(t *testing.T) {
	w := newWorld(t, location1)
	w.snap.Status.Volumes = nil
	base := "restora-mover-" + w.snap.Name + "-aaaa"
	w.advance()
	w.finish(testNamespace, base+"-r", 0, MoverReport{SnapshotID: "res-1", AddedBytes: 100})
	w.advance()
	if !w.advance() {
		t.Fatal("without volumes the copy is done after the resources")
	}
	record, err := w.records.Get(context.Background(), RecordName(w.snap.Name, location1))
	if err != nil || len(record.Spec.Volumes) != 0 || record.Spec.ResourcesSnapshot != "res-1" {
		t.Errorf("record = %+v, %v", record, err)
	}
}
