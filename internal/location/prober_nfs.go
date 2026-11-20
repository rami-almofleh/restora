package location

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"restora.local/restora/internal/snapshot"
)

const (
	// nfsTimeout bounds one test of an NFS location. The kubelet retries a failing mount for a long time, so the
	// controller gives up earlier and reports what the kubelet said.
	// A freshly started NFS server also holds back writes for a while after its start (the NFSv4 grace period).
	nfsTimeout      = 90 * time.Second
	nfsPollInterval = time.Second

	// ProbeCommand is the argument that makes the controller binary run the in-pod probe instead of the controller.
	ProbeCommand = "probe-dir"
	// probeMount is where the share is mounted in the test pod.
	probeMount = "/mnt/location"
	// probeUser is the user the test pod runs as, the same as the controller. The share must let it write.
	probeUser = 65532
)

// NFSProber tests an NFS share by starting a short-lived pod that mounts it, writes a file, reads it back and
// removes it. The kubelet does the mounting, so Restora needs no extra rights on the nodes.
type NFSProber struct {
	Client    kubernetes.Interface
	Namespace string
	// Image is the Restora controller image, which carries the probe command.
	Image      string
	PullPolicy corev1.PullPolicy
	// Timeout and Interval default to one minute and one second. Tests shorten them.
	Timeout  time.Duration
	Interval time.Duration
}

// Probe runs the test. It never returns an error: what went wrong is part of the Outcome.
func (p *NFSProber) Probe(ctx context.Context, l *LocationProfile) *Outcome {
	out := newOutcome(CheckMount, CheckWrite, CheckRead, CheckDelete)
	if l.Spec.NFS == nil {
		return out.fail(CheckMount, CodeTestFailed, "the location has no NFS settings", "Edit the location and fill in the NFS settings.")
	}
	timeout, interval := p.Timeout, p.Interval
	if timeout == 0 {
		timeout = nfsTimeout
	}
	if interval == 0 {
		interval = nfsPollInterval
	}

	pods := p.Client.CoreV1().Pods(p.Namespace)
	pod := p.pod(l)
	// A pod left over from an interrupted test would block the name.
	_ = pods.Delete(ctx, pod.Name, metav1.DeleteOptions{})
	if _, err := pods.Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return out.fail(CheckMount, CodeTestFailed, "an earlier test pod is still being removed", "Try again in a moment.")
		}
		return out.fail(CheckMount, CodeTestFailed, fmt.Sprintf("start the test pod: %v", err),
			"Check that the Restora controller may create pods in its namespace.")
	}
	defer func() {
		_ = pods.Delete(context.WithoutCancel(ctx), pod.Name, metav1.DeleteOptions{GracePeriodSeconds: new(int64)})
	}()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		current, err := pods.Get(ctx, pod.Name, metav1.GetOptions{})
		if err == nil {
			if done, result := finishedProbe(out, current); done {
				return result
			}
		}
		select {
		case <-ctx.Done():
			return out.fail(CheckMount, CodeTimeout, "the test was cancelled", "Try again.")
		case <-deadline.C:
			return p.timedOut(ctx, out, pod.Name)
		case <-ticker.C:
		}
	}
}

// finishedProbe reads the result of the test pod once its container has ended.
func finishedProbe(out *Outcome, pod *corev1.Pod) (bool, *Outcome) {
	for _, status := range pod.Status.ContainerStatuses {
		term := status.State.Terminated
		if term == nil {
			continue
		}
		out.pass(CheckMount)
		var report probeReport
		_ = json.Unmarshal([]byte(term.Message), &report)
		if term.ExitCode == 0 {
			out.pass(CheckWrite)
			out.pass(CheckRead)
			out.pass(CheckDelete)
			return true, out
		}
		stage := report.Stage
		if stage == "" {
			stage = CheckWrite
		}
		message := report.Error
		if message == "" {
			message = fmt.Sprintf("the test pod ended with exit code %d: %s", term.ExitCode, term.Reason)
		}
		// Steps before the failed one worked.
		for _, step := range []string{CheckWrite, CheckRead, CheckDelete} {
			if step == stage {
				break
			}
			out.pass(step)
		}
		code, hint := CodeTestFailed, "See the technical details."
		if stage == CheckWrite && (strings.Contains(message, "permission denied") || strings.Contains(message, "read-only")) {
			code = CodeNFSNotWritable
			hint = fmt.Sprintf("The share is mounted but user %d may not write to it. Change the permissions of the exported directory.", probeUser)
		}
		return true, out.fail(stage, code, message, hint)
	}
	if pod.Status.Phase == corev1.PodFailed {
		message := pod.Status.Message
		if message == "" {
			message = "the test pod failed: " + pod.Status.Reason
		}
		return true, out.fail(CheckMount, CodeNFSMountFailed, message, "Check the server, the path and the export rules of the NFS server.")
	}
	return false, nil
}

// timedOut explains a pod that did not finish. A mount that does not work leaves a FailedMount event with the reason.
// A pod that is running mounted the share, so the time went into writing.
func (p *NFSProber) timedOut(ctx context.Context, out *Outcome, podName string) *Outcome {
	if pod, err := p.Client.CoreV1().Pods(p.Namespace).Get(context.WithoutCancel(ctx), podName, metav1.GetOptions{}); err == nil &&
		pod.Status.Phase == corev1.PodRunning {
		out.pass(CheckMount)
		return out.fail(CheckWrite, CodeTimeout, "the share was mounted, but the test file was not written in time",
			"The NFS server answers very slowly. A server that has just started can take a minute; try again later.")
	}
	events, err := p.Client.CoreV1().Events(p.Namespace).List(context.WithoutCancel(ctx), metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + podName,
	})
	if err == nil {
		var newest *corev1.Event
		for i := range events.Items {
			e := &events.Items[i]
			if e.Type == corev1.EventTypeWarning && (e.Reason == "FailedMount" || e.Reason == "FailedAttachVolume") &&
				(newest == nil || e.LastTimestamp.After(newest.LastTimestamp.Time)) {
				newest = e
			}
		}
		if newest != nil {
			return out.fail(CheckMount, CodeNFSMountFailed, newest.Message,
				"Check the server, the path and the export rules of the NFS server, and that the nodes have an NFS client.")
		}
	}
	return out.fail(CheckMount, CodeTimeout, "the share was not mounted in time", "Check that the NFS server is reachable from the nodes of the cluster.")
}

func (p *NFSProber) pod(l *LocationProfile) *corev1.Pod {
	yes, no := true, false
	user := int64(probeUser)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restora-probe-" + l.Name,
			Namespace: p.Namespace,
			Labels: map[string]string{
				snapshot.LabelManagedBy: snapshot.ManagedByValue,
				"restora.local/probe":   l.Name,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: &no,
			ActiveDeadlineSeconds:        new(int64(120)),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   &yes,
				RunAsUser:      &user,
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{{
				Name:            "probe",
				Image:           p.Image,
				ImagePullPolicy: p.PullPolicy,
				Args:            []string{ProbeCommand, probeMount},
				VolumeMounts:    []corev1.VolumeMount{{Name: "share", MountPath: probeMount}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("16Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Mi")},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &no,
					ReadOnlyRootFilesystem:   &yes,
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
			}},
			Volumes: []corev1.Volume{{
				Name: "share",
				VolumeSource: corev1.VolumeSource{
					NFS: &corev1.NFSVolumeSource{Server: l.Spec.NFS.Server, Path: l.Spec.NFS.Path},
				},
			}},
		},
	}
}
