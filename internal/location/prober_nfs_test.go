package location

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
)

func nfsProfile() *LocationProfile {
	return &LocationProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "l-1"},
		Spec:       Spec{Name: "Share", Type: TypeNFS, NFS: &NFSSpec{Server: "nfs.example.com", Path: "/exports/restora"}},
	}
}

func newNFSProber(client *kubefake.Clientset) *NFSProber {
	return &NFSProber{
		Client: client, Namespace: testNamespace, Image: "restora-controller:dev", PullPolicy: corev1.PullIfNotPresent,
		Timeout: 300 * time.Millisecond, Interval: 10 * time.Millisecond,
	}
}

// podEnds makes every pod that is created end at once with the given exit code and termination message.
func podEnds(client *kubefake.Clientset, exitCode int32, message string) {
	client.PrependReactor("create", "pods", func(action kubetesting.Action) (bool, runtime.Object, error) {
		pod := action.(kubetesting.CreateAction).GetObject().(*corev1.Pod).DeepCopy()
		pod.Status.Phase = corev1.PodSucceeded
		if exitCode != 0 {
			pod.Status.Phase = corev1.PodFailed
		}
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: "probe",
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				ExitCode: exitCode, Message: message, Reason: "Completed",
			}},
		}}
		_ = client.Tracker().Add(pod)
		return true, pod, nil
	})
}

func podsLeft(t *testing.T, client *kubefake.Clientset) int {
	t.Helper()
	list, err := client.CoreV1().Pods(testNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

func TestNFSProbeSucceeds(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	podEnds(client, 0, `{}`)

	out := newNFSProber(client).Probe(context.Background(), nfsProfile())

	if !out.OK() || checkPhases(out) != "mount=Succeeded write=Succeeded read=Succeeded delete=Succeeded" {
		t.Errorf("outcome = %+v (%s)", out, checkPhases(out))
	}
	if n := podsLeft(t, client); n != 0 {
		t.Errorf("pods left = %d; the test pod must be removed", n)
	}
}

func TestNFSProbePodMountsTheShareSafely(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	var created *corev1.Pod
	client.PrependReactor("create", "pods", func(action kubetesting.Action) (bool, runtime.Object, error) {
		created = action.(kubetesting.CreateAction).GetObject().(*corev1.Pod).DeepCopy()
		return false, nil, nil
	})
	newNFSProber(client).Probe(context.Background(), nfsProfile())

	if created == nil {
		t.Fatal("no pod was created")
	}
	nfs := created.Spec.Volumes[0].NFS
	if nfs == nil || nfs.Server != "nfs.example.com" || nfs.Path != "/exports/restora" || nfs.ReadOnly {
		t.Errorf("volume = %+v", created.Spec.Volumes[0])
	}
	c := created.Spec.Containers[0]
	if c.Image != "restora-controller:dev" || strings.Join(c.Args, " ") != ProbeCommand+" "+probeMount {
		t.Errorf("container = %s %v", c.Image, c.Args)
	}
	if *created.Spec.AutomountServiceAccountToken || !*created.Spec.SecurityContext.RunAsNonRoot || *created.Spec.SecurityContext.RunAsUser != probeUser ||
		*c.SecurityContext.AllowPrivilegeEscalation || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Errorf("the test pod must run unprivileged and without cluster access: %+v", created.Spec)
	}
}

func TestNFSProbeReportsWhatTheProbeFound(t *testing.T) {
	tests := []struct {
		name     string
		exit     int32
		message  string
		wantCode string
		wantRun  string
	}{
		{"share is read-only for the user", 1, `{"stage":"write","error":"open /mnt/location/.x: permission denied"}`,
			CodeNFSNotWritable, "mount=Succeeded write=Failed read=Pending delete=Pending"},
		{"read fails", 1, `{"stage":"read","error":"input/output error"}`,
			CodeTestFailed, "mount=Succeeded write=Succeeded read=Failed delete=Pending"},
		{"delete fails", 1, `{"stage":"delete","error":"operation not permitted"}`,
			CodeTestFailed, "mount=Succeeded write=Succeeded read=Succeeded delete=Failed"},
		{"no report", 2, ``, CodeTestFailed, "mount=Succeeded write=Failed read=Pending delete=Pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := kubefake.NewSimpleClientset()
			podEnds(client, tt.exit, tt.message)

			out := newNFSProber(client).Probe(context.Background(), nfsProfile())

			if out.Code != tt.wantCode || out.Message == "" || out.Hint == "" || checkPhases(out) != tt.wantRun {
				t.Errorf("outcome = %+v (%s)", out, checkPhases(out))
			}
			if n := podsLeft(t, client); n != 0 {
				t.Errorf("pods left = %d", n)
			}
		})
	}
}

func TestNFSProbeMountNeverWorks(t *testing.T) {
	client := kubefake.NewSimpleClientset(&corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "e1", Namespace: testNamespace},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "restora-probe-l-1"},
		Type:           corev1.EventTypeWarning, Reason: "FailedMount",
		Message: "MountVolume.SetUp failed for volume \"share\": mount.nfs: Connection timed out",
	})

	out := newNFSProber(client).Probe(context.Background(), nfsProfile())

	if out.Code != CodeNFSMountFailed || !strings.Contains(out.Message, "Connection timed out") {
		t.Errorf("outcome = %+v", out)
	}
	if checkPhases(out) != "mount=Failed write=Pending read=Pending delete=Pending" {
		t.Errorf("checks = %s", checkPhases(out))
	}
	if n := podsLeft(t, client); n != 0 {
		t.Errorf("pods left = %d; a pod that never started must be removed too", n)
	}
}

func TestNFSProbeTimesOutWithoutAReason(t *testing.T) {
	out := newNFSProber(kubefake.NewSimpleClientset()).Probe(context.Background(), nfsProfile())
	if out.Code != CodeTimeout {
		t.Errorf("outcome = %+v, want %s", out, CodeTimeout)
	}
}

func TestNFSProbeShareMountedButWritingHangs(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	client.PrependReactor("create", "pods", func(action kubetesting.Action) (bool, runtime.Object, error) {
		pod := action.(kubetesting.CreateAction).GetObject().(*corev1.Pod).DeepCopy()
		pod.Status.Phase = corev1.PodRunning
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "probe", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}
		_ = client.Tracker().Add(pod)
		return true, pod, nil
	})

	out := newNFSProber(client).Probe(context.Background(), nfsProfile())

	if out.Code != CodeTimeout || checkPhases(out) != "mount=Succeeded write=Failed read=Pending delete=Pending" {
		t.Errorf("outcome = %+v (%s)", out, checkPhases(out))
	}
	if n := podsLeft(t, client); n != 0 {
		t.Errorf("pods left = %d", n)
	}
}

func TestNFSProbeClearsAPodLeftBehind(t *testing.T) {
	client := kubefake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "restora-probe-l-1", Namespace: testNamespace}})
	podEnds(client, 0, `{}`)

	if out := newNFSProber(client).Probe(context.Background(), nfsProfile()); !out.OK() {
		t.Errorf("outcome = %+v", out)
	}
}

func TestNFSProbeWithoutSettings(t *testing.T) {
	out := newNFSProber(kubefake.NewSimpleClientset()).Probe(context.Background(), &LocationProfile{Spec: Spec{Type: TypeNFS}})
	if out.OK() {
		t.Error("a location without NFS settings cannot work")
	}
}

func TestProbeDirectory(t *testing.T) {
	dir := t.TempDir()
	if stage, err := ProbeDirectory(dir); err != nil || stage != "" {
		t.Fatalf("ProbeDirectory = %q, %v", stage, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the directory holds %d files afterwards, want none", len(entries))
	}

	stage, err := ProbeDirectory(dir + "/does-not-exist")
	if err == nil || stage != CheckWrite {
		t.Errorf("a directory that is not there: stage = %q, error = %v", stage, err)
	}
}

func TestRunProbeWritesTheReport(t *testing.T) {
	log := t.TempDir() + "/termination-log"
	if code := RunProbe(t.TempDir(), log); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if data, _ := os.ReadFile(log); string(data) != `{}` {
		t.Errorf("report = %s", data)
	}

	if code := RunProbe(t.TempDir()+"/missing", log); code == 0 {
		t.Fatal("a failing probe must exit with an error")
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), `"stage":"write"`) || !strings.Contains(string(data), "error") {
		t.Errorf("report = %s", data)
	}
}
