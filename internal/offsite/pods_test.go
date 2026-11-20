package offsite

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func newMover() (*Mover, *kubefake.Clientset) {
	kube := kubefake.NewSimpleClientset()
	return &Mover{Client: kube, Image: "restora-controller:dev", PullPolicy: corev1.PullIfNotPresent}, kube
}

func TestStartIsSafeToRepeat(t *testing.T) {
	m, _ := newMover()
	job := Job{Name: "mover-1", Namespace: "demo", Work: "s-1", Kind: "backup", Target: s3Target(), Host: "demo", Claim: "data", ReadOnly: true}
	if err := m.Start(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background(), job); err != nil {
		t.Errorf("starting a job that exists must not fail: %v", err)
	}
}

func TestMoverPodRunsAsRootWithAReadOnlyVolume(t *testing.T) {
	m, kube := newMover()
	job := Job{Name: "mover-1", Namespace: "demo", Work: "s-1", Kind: "backup", Target: nfsTarget(), Host: "demo", Claim: "data", ReadOnly: true}
	if err := m.Start(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	pod, _ := kube.CoreV1().Pods("demo").Get(context.Background(), "mover-1", metav1.GetOptions{})
	c := pod.Spec.Containers[0]
	if *c.SecurityContext.RunAsUser != 0 || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Errorf("security context = %+v", c.SecurityContext)
	}
	if pod.Labels[LabelMover] != "s-1" || pod.Spec.RestartPolicy != corev1.RestartPolicyNever || *pod.Spec.AutomountServiceAccountToken {
		t.Errorf("pod = %+v", pod.ObjectMeta)
	}
	volumes := map[string]corev1.Volume{}
	for _, v := range pod.Spec.Volumes {
		volumes[v.Name] = v
	}
	if data := volumes["data"]; data.PersistentVolumeClaim == nil || !data.PersistentVolumeClaim.ReadOnly || data.PersistentVolumeClaim.ClaimName != "mover-1" {
		t.Errorf("a backup reads the temporary volume, which is named like the job: %+v", data)
	}
	if repo := volumes["repository"]; repo.NFS == nil || repo.NFS.Server != "nfs" || repo.NFS.Path != "/exports" {
		t.Errorf("the NFS location is mounted by the pod: %+v", repo)
	}
	mounts := map[string]string{}
	for _, m := range c.VolumeMounts {
		mounts[m.Name] = m.MountPath
	}
	if mounts["data"] != "/volumes/data" || mounts["repository"] != NFSMountPath {
		t.Errorf("mounts = %v", mounts)
	}
}

func TestMoverPodForAnS3LocationNeedsNoShare(t *testing.T) {
	m, kube := newMover()
	job := Job{Name: "mover-r", Namespace: "restora-system", Work: "s-1", Kind: "backup", Target: s3Target(), Host: "demo",
		Bundle: map[string][]byte{"resources.json.gz": []byte("x")}}
	if err := m.Start(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	pod, _ := kube.CoreV1().Pods("restora-system").Get(context.Background(), "mover-r", metav1.GetOptions{})
	for _, v := range pod.Spec.Volumes {
		if v.NFS != nil || v.PersistentVolumeClaim != nil {
			t.Errorf("volume %s is not needed for S3 and the saved resources", v.Name)
		}
	}
	var dir string
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == envMoverDir {
			dir = e.Value
		}
	}
	if dir != "/bundle" {
		t.Errorf("dir = %q", dir)
	}
}

func TestStatusOfAPod(t *testing.T) {
	m, kube := newMover()
	ctx := context.Background()
	if out, err := m.Status(ctx, "demo", "none"); err != nil || out.State != StateMissing {
		t.Errorf("missing pod: %+v, %v", out, err)
	}

	pending := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "demo"}, Status: corev1.PodStatus{
		Phase:      corev1.PodPending,
		Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "persistentvolumeclaim not found"}},
	}}
	running := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "demo"}, Status: corev1.PodStatus{
		ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
	}}
	crashed := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "crashed", Namespace: "demo"}, Status: corev1.PodStatus{
		ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Reason: "OOMKilled"}}}},
	}}
	for _, p := range []*corev1.Pod{pending, running, crashed} {
		if _, err := kube.CoreV1().Pods("demo").Create(ctx, p, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	if out, _ := m.Status(ctx, "demo", "pending"); out.State != StateRunning || out.Started || out.NotStartedReason == "" {
		t.Errorf("pending: %+v; an unschedulable pod must say why", out)
	}
	if out, _ := m.Status(ctx, "demo", "running"); out.State != StateRunning || !out.Started {
		t.Errorf("running: %+v", out)
	}
	out, _ := m.Status(ctx, "demo", "crashed")
	if out.State != StateFailed || out.Report.Error == "" || out.Report.Code != CodeFailed {
		t.Errorf("a pod that died without a report is still a failure: %+v", out)
	}
}

func TestRemoveDeletesWhatAJobCreated(t *testing.T) {
	m, kube := newMover()
	ctx := context.Background()
	job := Job{Name: "mover-1", Namespace: "demo", Work: "s-1", Kind: "backup", Target: s3Target(), Host: "demo", Claim: "data", ReadOnly: true}
	if err := m.EnsureClaim(ctx, job, "s-1-0", 1<<30, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(ctx, job); err != nil {
		t.Fatal(err)
	}
	pods, _ := kube.CoreV1().Pods("demo").List(ctx, metav1.ListOptions{})
	secrets, _ := kube.CoreV1().Secrets("demo").List(ctx, metav1.ListOptions{})
	claims, _ := kube.CoreV1().PersistentVolumeClaims("demo").List(ctx, metav1.ListOptions{})
	if len(pods.Items)+len(secrets.Items)+len(claims.Items) != 0 {
		t.Errorf("left: %d pods, %d secrets, %d claims", len(pods.Items), len(secrets.Items), len(claims.Items))
	}
	if err := m.Remove(ctx, job); err != nil {
		t.Errorf("removing what is gone must not fail: %v", err)
	}
}

func TestRemoveKeepsTheClaimOfARestore(t *testing.T) {
	m, kube := newMover()
	ctx := context.Background()
	job := Job{Name: "mover-v0", Namespace: "demo", Work: "r-1", Kind: "restore", Target: s3Target(), Host: "demo", Claim: "data", SnapshotID: "vol-1"}
	if _, err := kube.CoreV1().PersistentVolumeClaims("demo").Create(ctx, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "demo"}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(ctx, job); err != nil {
		t.Fatal(err)
	}
	if _, err := kube.CoreV1().PersistentVolumeClaims("demo").Get(ctx, "data", metav1.GetOptions{}); err != nil {
		t.Errorf("the claim of the application is the result of the restore and must stay: %v", err)
	}
}
