package offsite

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"restora.local/restora/internal/snapshot"
)

const (
	// PasswordSecretPrefix names the secret that holds the key of a repository: restora-repo-<location>.
	PasswordSecretPrefix = "restora-repo-"
	passwordKey          = "password"
)

// PasswordSecretName is the name of the secret with the key of a location's repository.
func PasswordSecretName(location string) string {
	return PasswordSecretPrefix + location
}

// Passwords keeps the keys that encrypt the repositories. A key is made once, when the first backup goes to a
// location, and never replaced: without it the stored backups cannot be read again.
//
// The secret does not belong to the location profile, so deleting the profile does not delete the key.
type Passwords struct {
	Client    kubernetes.Interface
	Namespace string
}

// ErrNoKey is returned when the key of a repository is not in the cluster, for example in a new cluster.
var ErrNoKey = errors.New("the repository key is not in this cluster")

// Existing returns the key of the location's repository. Unlike Get it never makes a new one: a restore with a
// freshly made key could only fail, and would hide that the real key is missing.
func (p *Passwords) Existing(ctx context.Context, location string) (string, error) {
	secret, err := p.Client.CoreV1().Secrets(p.Namespace).Get(ctx, PasswordSecretName(location), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", fmt.Errorf("%w: secret %s does not exist", ErrNoKey, PasswordSecretName(location))
	}
	if err != nil {
		return "", fmt.Errorf("read secret %s: %w", PasswordSecretName(location), err)
	}
	if pw := string(secret.Data[passwordKey]); pw != "" {
		return pw, nil
	}
	return "", fmt.Errorf("%w: secret %s has no %s", ErrNoKey, PasswordSecretName(location), passwordKey)
}

// Get returns the key of the location's repository and creates it first when it does not exist.
func (p *Passwords) Get(ctx context.Context, location string) (string, error) {
	secrets := p.Client.CoreV1().Secrets(p.Namespace)
	name := PasswordSecretName(location)
	if secret, err := secrets.Get(ctx, name, metav1.GetOptions{}); err == nil {
		if pw := string(secret.Data[passwordKey]); pw != "" {
			return pw, nil
		}
		return "", fmt.Errorf("secret %s has no %s", name, passwordKey)
	} else if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("read secret %s: %w", name, err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make repository key: %w", err)
	}
	password := hex.EncodeToString(raw)
	_, err := secrets.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: p.Namespace,
			Labels:      map[string]string{snapshot.LabelManagedBy: snapshot.ManagedByValue},
			Annotations: map[string]string{"restora.local/note": "Encrypts the backups in the storage location. Keep a copy: without it they cannot be read."},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{passwordKey: []byte(password)},
	}, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return p.Get(ctx, location) // another pass was faster: use its key
	}
	if err != nil {
		return "", fmt.Errorf("store repository key: %w", err)
	}
	return password, nil
}
