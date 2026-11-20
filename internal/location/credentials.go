package location

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"restora.local/restora/internal/snapshot"
)

// ErrCredentialsMissing is returned when the secret of a location does not exist or lacks a key.
var ErrCredentialsMissing = errors.New("credentials missing")

// SecretName is the name of the secret the API creates for a location.
func SecretName(locationID string) string {
	return SecretPrefix + locationID
}

// SecretWriter stores keys as a Kubernetes secret. It can write but never read: the API must not get credentials
// back once they are stored.
type SecretWriter struct {
	client    kubernetes.Interface
	namespace string
}

// NewSecretWriter creates a SecretWriter for secrets in one namespace.
func NewSecretWriter(client kubernetes.Interface, namespace string) *SecretWriter {
	return &SecretWriter{client: client, namespace: namespace}
}

// Put creates or replaces the secret of a location. The secret belongs to the profile, so deleting the profile
// deletes the secret too.
func (w *SecretWriter) Put(ctx context.Context, owner *LocationProfile, accessKey, secretKey string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(owner.Name),
			Namespace: w.namespace,
			Labels:    map[string]string{snapshot.LabelManagedBy: snapshot.ManagedByValue},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: GVR.GroupVersion().String(),
				Kind:       Kind,
				Name:       owner.Name,
				UID:        owner.UID,
			}},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{SecretKeyAccess: accessKey, SecretKeySecret: secretKey},
	}
	secrets := w.client.CoreV1().Secrets(w.namespace)
	_, err := secrets.Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// An update without a resource version replaces the secret, and needs no right to read it.
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("store credentials of location %s: %w", owner.Name, err)
	}
	return nil
}

// SecretReader reads the keys of a location. Only the controller uses it.
type SecretReader struct {
	client    kubernetes.Interface
	namespace string
}

// NewSecretReader creates a SecretReader for secrets in one namespace.
func NewSecretReader(client kubernetes.Interface, namespace string) *SecretReader {
	return &SecretReader{client: client, namespace: namespace}
}

// Credentials returns the access key and the secret key stored in the named secret.
func (r *SecretReader) Credentials(ctx context.Context, secretName string) (accessKey, secretKey string, err error) {
	if secretName == "" {
		return "", "", fmt.Errorf("%w: the location names no secret", ErrCredentialsMissing)
	}
	secret, err := r.client.CoreV1().Secrets(r.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", "", fmt.Errorf("%w: secret %s does not exist", ErrCredentialsMissing, secretName)
	}
	if err != nil {
		return "", "", fmt.Errorf("read secret %s: %w", secretName, err)
	}
	accessKey, secretKey = string(secret.Data[SecretKeyAccess]), string(secret.Data[SecretKeySecret])
	if accessKey == "" || secretKey == "" {
		return "", "", fmt.Errorf("%w: secret %s needs the keys %s and %s", ErrCredentialsMissing, secretName, SecretKeyAccess, SecretKeySecret)
	}
	return accessKey, secretKey, nil
}
