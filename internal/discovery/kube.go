package discovery

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
)

// Secrets and config maps are read through the metadata client, so their content never reaches Restora.
// Service account tokens, Helm release records and the automatic root CA map are not application data.
const (
	secretFilter    = "type!=kubernetes.io/service-account-token,type!=helm.sh/release.v1"
	configMapFilter = "metadata.name!=kube-root-ca.crt"
)

// KubeSource reads from a live Kubernetes cluster.
type KubeSource struct {
	typed kubernetes.Interface
	meta  metadata.Interface
}

// NewKubeSource creates a KubeSource from a client configuration.
func NewKubeSource(cfg *rest.Config) (*KubeSource, error) {
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	meta, err := metadata.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create metadata client: %w", err)
	}
	return &KubeSource{typed: typed, meta: meta}, nil
}

// Namespaces implements Source.
func (k *KubeSource) Namespaces(ctx context.Context) ([]NamespaceObj, error) {
	list, err := k.typed.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]NamespaceObj, 0, len(list.Items))
	for _, ns := range list.Items {
		out = append(out, NamespaceObj{Name: ns.Name, Labels: ns.Labels, CreatedAt: ns.CreationTimestamp.Time})
	}
	return out, nil
}

// Workloads implements Source and returns Deployments and StatefulSets.
func (k *KubeSource) Workloads(ctx context.Context) ([]WorkloadObj, error) {
	deployments, err := k.typed.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("deployments: %w", err)
	}
	statefulSets, err := k.typed.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("statefulsets: %w", err)
	}

	out := make([]WorkloadObj, 0, len(deployments.Items)+len(statefulSets.Items))
	for _, d := range deployments.Items {
		out = append(out, WorkloadObj{Namespace: d.Namespace, Workload: Workload{
			Kind:    "Deployment",
			Name:    d.Name,
			Ready:   d.Status.ReadyReplicas,
			Desired: desiredReplicas(d.Spec.Replicas),
			Images:  containerImages(d.Spec.Template.Spec),
		}})
	}
	for _, s := range statefulSets.Items {
		out = append(out, WorkloadObj{Namespace: s.Namespace, Workload: Workload{
			Kind:    "StatefulSet",
			Name:    s.Name,
			Ready:   s.Status.ReadyReplicas,
			Desired: desiredReplicas(s.Spec.Replicas),
			Images:  containerImages(s.Spec.Template.Spec),
		}})
	}
	return out, nil
}

// Volumes implements Source.
func (k *KubeSource) Volumes(ctx context.Context) ([]VolumeObj, error) {
	list, err := k.typed.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]VolumeObj, 0, len(list.Items))
	for _, pvc := range list.Items {
		v := Volume{
			Name:          pvc.Name,
			CapacityBytes: pvcCapacity(pvc),
			Phase:         string(pvc.Status.Phase),
		}
		if pvc.Spec.StorageClassName != nil {
			v.StorageClass = *pvc.Spec.StorageClassName
		}
		out = append(out, VolumeObj{Namespace: pvc.Namespace, Volume: v})
	}
	return out, nil
}

// ConfigMaps implements Source.
func (k *KubeSource) ConfigMaps(ctx context.Context) ([]NamedObj, error) {
	return k.names(ctx, corev1.SchemeGroupVersion.WithResource("configmaps"), configMapFilter)
}

// Secrets implements Source.
func (k *KubeSource) Secrets(ctx context.Context) ([]NamedObj, error) {
	return k.names(ctx, corev1.SchemeGroupVersion.WithResource("secrets"), secretFilter)
}

func (k *KubeSource) names(ctx context.Context, gvr schema.GroupVersionResource, fieldSelector string) ([]NamedObj, error) {
	list, err := k.meta.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		return nil, err
	}
	out := make([]NamedObj, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, NamedObj{Namespace: item.Namespace, Name: item.Name})
	}
	return out, nil
}

// desiredReplicas applies the Kubernetes default of one replica.
func desiredReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

func containerImages(spec corev1.PodSpec) []string {
	images := make([]string, 0, len(spec.Containers))
	for _, c := range spec.Containers {
		images = append(images, c.Image)
	}
	return images
}

// pvcCapacity prefers the real size and falls back to the requested one while the claim is pending.
func pvcCapacity(pvc corev1.PersistentVolumeClaim) int64 {
	if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		return q.Value()
	}
	if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		return q.Value()
	}
	return 0
}
