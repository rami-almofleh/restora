package offsite

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"restora.local/restora/internal/snapshot"
)

const (
	// LabelMover marks everything a data mover needs (pod, secrets, temporary volume). Its value names what the mover
	// works for, so one call removes it all.
	LabelMover = "restora.local/mover"

	volumesMount = "/volumes"
	bundleMount  = "/bundle"
	// maxDeadlineSeconds ends a data mover that hangs. A copy of a large volume can take hours.
	maxDeadlineSeconds = 24 * 60 * 60
)

// Mover starts data mover pods and reads how they ended. The pods run the Restora image, which has Kopia.
type Mover struct {
	Client     kubernetes.Interface
	Image      string
	PullPolicy corev1.PullPolicy
}

// Job is one run of a data mover.
type Job struct {
	// Name is shared by the pod and its secrets.
	Name      string
	Namespace string
	// Work is what the job belongs to, for example a snapshot or a restore. Cleanup works on it.
	Work string
	// Kind is "backup" or "restore".
	Kind   string
	Target Target
	// Host is the identity of the snapshots in the repository.
	Host        string
	Tags        map[string]string
	Description string
	// SnapshotID is the snapshot to restore.
	SnapshotID string

	// Claim is the volume claim the pod mounts at /volumes/<Claim>. Empty when the job works on Bundle.
	Claim string
	// ReadOnly mounts the claim read-only, as a backup does.
	ReadOnly bool
	// Bundle holds files the pod mounts at /bundle, for the saved resources.
	Bundle map[string][]byte
}

// State of a data mover pod.
type State string

const (
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateMissing   State = "missing"
)

// Outcome is how a data mover pod stands.
type Outcome struct {
	State  State
	Report MoverReport
	// Started is true once a container ran. CreatedAt is when the pod was created.
	Started   bool
	CreatedAt time.Time
	// NotStartedReason says why the pod has not started, for example ImagePullBackOff or why it cannot be scheduled.
	NotStartedReason string
}

func (j Job) envSecretName() string    { return j.Name + "-env" }
func (j Job) bundleSecretName() string { return j.Name + "-bundle" }

func (j Job) dir() string {
	if j.Kind == "fetch-bundle" {
		return BundleDir
	}
	if j.Bundle != nil {
		return bundleMount
	}
	return volumesMount + "/" + j.Claim
}

func (j Job) labels() map[string]string {
	return map[string]string{snapshot.LabelManagedBy: snapshot.ManagedByValue, LabelMover: j.Work}
}

// Start creates the secrets and the pod of the job. Starting a job that exists already changes nothing.
func (m *Mover) Start(ctx context.Context, j Job) error {
	env := map[string]string{}
	for k, v := range j.Target.Env() {
		env[k] = v
	}
	if err := m.ensureSecret(ctx, j, j.envSecretName(), env, nil); err != nil {
		return err
	}
	if j.Bundle != nil {
		if err := m.ensureSecret(ctx, j, j.bundleSecretName(), nil, j.Bundle); err != nil {
			return err
		}
	}
	_, err := m.Client.CoreV1().Pods(j.Namespace).Create(ctx, m.pod(j), metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("start data mover %s: %w", j.Name, err)
	}
	return nil
}

func (m *Mover) ensureSecret(ctx context.Context, j Job, name string, stringData map[string]string, data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: j.Namespace, Labels: j.labels()},
		Type:       corev1.SecretTypeOpaque,
		StringData: stringData,
		Data:       data,
	}
	_, err := m.Client.CoreV1().Secrets(j.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create secret %s: %w", name, err)
	}
	return nil
}

func (m *Mover) pod(j Job) *corev1.Pod {
	no := false
	deadline := int64(maxDeadlineSeconds)
	env := []corev1.EnvVar{
		{Name: envMoverDir, Value: j.dir()},
		{Name: envMoverHost, Value: j.Host},
		{Name: envMoverTags, Value: formatTags(j.Tags)},
		{Name: envMoverDescription, Value: j.Description},
		{Name: envMoverSnapshot, Value: j.SnapshotID},
	}
	container := corev1.Container{
		Name:            "mover",
		Image:           m.Image,
		ImagePullPolicy: m.PullPolicy,
		Args:            []string{MoverCommand, j.Kind},
		Env:             env,
		EnvFrom:         []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: j.envSecretName()}}}},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
		},
		// A data mover reads and writes files of every owner, so it runs as root with the default capabilities.
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                new(int64(0)),
			AllowPrivilegeEscalation: &no,
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}
	var volumes []corev1.Volume
	if j.Claim != "" {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "data", MountPath: j.dir()})
		volumes = append(volumes, corev1.Volume{Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: j.pvcName(), ReadOnly: j.ReadOnly},
		}})
	}
	if j.Bundle != nil {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "bundle", MountPath: bundleMount, ReadOnly: true})
		volumes = append(volumes, corev1.Volume{Name: "bundle", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: j.bundleSecretName()},
		}})
	}
	if j.Target.NFS != nil {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "repository", MountPath: NFSMountPath})
		volumes = append(volumes, corev1.Volume{Name: "repository", VolumeSource: corev1.VolumeSource{
			NFS: &corev1.NFSVolumeSource{Server: j.Target.NFS.Server, Path: j.Target.NFS.Path},
		}})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: j.Name, Namespace: j.Namespace, Labels: j.labels()},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: &no,
			ActiveDeadlineSeconds:        &deadline,
			Containers:                   []corev1.Container{container},
			Volumes:                      volumes,
		},
	}
}

// pvcName is the volume claim a job mounts. A backup works on a temporary claim made from the snapshot, a restore on
// the claim of the application. The caller names it through Claim for restores and through Name for backups.
func (j Job) pvcName() string {
	if j.Kind == "backup" {
		return j.Name
	}
	return j.Claim
}

// Status reads how the pod of a job stands.
func (m *Mover) Status(ctx context.Context, namespace, name string) (Outcome, error) {
	pod, err := m.Client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return Outcome{State: StateMissing}, nil
	}
	if err != nil {
		return Outcome{}, fmt.Errorf("get data mover %s: %w", name, err)
	}
	for _, c := range pod.Status.ContainerStatuses {
		if term := c.State.Terminated; term != nil {
			report := MoverReport{}
			_ = json.Unmarshal([]byte(term.Message), &report)
			state := StateSucceeded
			if term.ExitCode != 0 || report.Error != "" {
				state = StateFailed
				if report.Error == "" {
					report.Code = CodeFailed
					report.Error = fmt.Sprintf("the data mover ended with exit code %d (%s)", term.ExitCode, term.Reason)
				}
			}
			return Outcome{State: state, Report: report, Started: true, CreatedAt: pod.CreationTimestamp.Time}, nil
		}
		if c.State.Running != nil {
			return Outcome{State: StateRunning, Started: true, CreatedAt: pod.CreationTimestamp.Time}, nil
		}
	}
	if pod.Status.Phase == corev1.PodFailed {
		return Outcome{State: StateFailed, Report: MoverReport{Code: CodeFailed, Error: "the data mover pod failed: " + pod.Status.Reason + " " + pod.Status.Message}}, nil
	}
	return Outcome{State: StateRunning, NotStartedReason: notStartedReason(pod), CreatedAt: pod.CreationTimestamp.Time}, nil
}

// notStartedReason explains a pod whose container did not start: a container that waits for an image or a setting,
// or a pod that cannot be placed, for example because its volume cannot be created.
func notStartedReason(pod *corev1.Pod) string {
	for _, c := range pod.Status.ContainerStatuses {
		if w := c.State.Waiting; w != nil && w.Reason != "ContainerCreating" && w.Reason != "PodInitializing" {
			return w.Reason + ": " + w.Message
		}
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			return c.Reason + ": " + c.Message
		}
	}
	return ""
}

// Remove deletes the pod, the secrets and the temporary volume of one job.
func (m *Mover) Remove(ctx context.Context, j Job) error {
	var errs []error
	zero := metav1.DeleteOptions{GracePeriodSeconds: new(int64)}
	if err := m.Client.CoreV1().Pods(j.Namespace).Delete(ctx, j.Name, zero); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("delete data mover %s: %w", j.Name, err))
	}
	for _, name := range []string{j.envSecretName(), j.bundleSecretName()} {
		if err := m.Client.CoreV1().Secrets(j.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete secret %s: %w", name, err))
		}
	}
	if j.Kind == "backup" && j.Claim != "" {
		if err := m.Client.CoreV1().PersistentVolumeClaims(j.Namespace).Delete(ctx, j.pvcName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete temporary volume %s: %w", j.pvcName(), err))
		}
	}
	return joinErrors(errs)
}

// EnsureClaim creates a temporary volume claim for a backup, filled from a VolumeSnapshot.
func (m *Mover) EnsureClaim(ctx context.Context, j Job, volumeSnapshot string, sizeBytes int64, storageClass *string, accessModes []corev1.PersistentVolumeAccessMode) error {
	group := "snapshot.storage.k8s.io"
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: j.pvcName(), Namespace: j.Namespace, Labels: j.labels()},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      accessModes,
			StorageClassName: storageClass,
			DataSource:       &corev1.TypedLocalObjectReference{APIGroup: &group, Kind: "VolumeSnapshot", Name: volumeSnapshot},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: *resource.NewQuantity(sizeBytes, resource.BinarySI)},
			},
		},
	}
	_, err := m.Client.CoreV1().PersistentVolumeClaims(j.Namespace).Create(ctx, claim, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create temporary volume %s: %w", claim.Name, err)
	}
	return nil
}

// Cleanup removes everything the data movers of a piece of work left: pods, secrets and temporary volumes.
func (m *Mover) Cleanup(ctx context.Context, namespace, work string) error {
	selector := metav1.ListOptions{LabelSelector: LabelMover + "=" + work}
	zero := metav1.DeleteOptions{GracePeriodSeconds: new(int64)}
	var errs []error
	note := func(kind, name string, err error) {
		if err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete %s %s: %w", kind, name, err))
		}
	}

	pods, err := m.Client.CoreV1().Pods(namespace).List(ctx, selector)
	if err != nil {
		return fmt.Errorf("list data mover pods: %w", err)
	}
	for _, p := range pods.Items {
		note("data mover", p.Name, m.Client.CoreV1().Pods(namespace).Delete(ctx, p.Name, zero))
	}
	secrets, err := m.Client.CoreV1().Secrets(namespace).List(ctx, selector)
	if err != nil {
		return fmt.Errorf("list data mover secrets: %w", err)
	}
	for _, s := range secrets.Items {
		note("secret", s.Name, m.Client.CoreV1().Secrets(namespace).Delete(ctx, s.Name, metav1.DeleteOptions{}))
	}
	claims, err := m.Client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, selector)
	if err != nil {
		return fmt.Errorf("list temporary volumes: %w", err)
	}
	for _, c := range claims.Items {
		note("temporary volume", c.Name, m.Client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, c.Name, metav1.DeleteOptions{}))
	}
	return joinErrors(errs)
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	sort.SliceStable(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
	msg := errs[0].Error()
	for _, e := range errs[1:] {
		msg += "; " + e.Error()
	}
	return fmt.Errorf("%s", msg)
}
