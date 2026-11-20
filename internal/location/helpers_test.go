package location

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

const testNamespace = "restora-system"

var testNow = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newStore() *Store {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{GVR: "LocationProfileList"})
	return NewStore(client, testNamespace)
}

// world is a service with its stores: profiles in a fake dynamic client, secrets in a fake typed client.
type world struct {
	svc     *Service
	store   *Store
	kube    *kubefake.Clientset
	secrets *SecretWriter
}

func newWorld() *world {
	store := newStore()
	kube := kubefake.NewSimpleClientset()
	writer := NewSecretWriter(kube, testNamespace)
	svc := NewService(store, writer)
	svc.now = func() time.Time { return testNow }
	return &world{svc: svc, store: store, kube: kube, secrets: writer}
}

func s3Input(name string) Input {
	return Input{
		Name: name,
		Type: TypeS3,
		S3: &S3Input{
			Endpoint: "http://minio.minio:9000", Bucket: "restora", PathStyle: true,
			AccessKey: "AKIATEST", SecretKey: "very-secret-value",
		},
	}
}

func nfsInput(name string) Input {
	return Input{Name: name, Type: TypeNFS, NFS: &NFSSpec{Server: "nfs.example.com", Path: "/exports/" + strings.ToLower(name)}}
}

func mustCreate(t *testing.T, svc *Service, in Input) View {
	t.Helper()
	v, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return v
}
