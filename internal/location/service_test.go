package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
	"restora.local/restora/internal/snapshot"
)

func secretOf(t *testing.T, w *world, id string) map[string]string {
	t.Helper()
	secret, err := w.kube.CoreV1().Secrets(testNamespace).Get(context.Background(), SecretName(id), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret of %s: %v", id, err)
	}
	out := map[string]string{}
	for k, v := range secret.StringData {
		out[k] = v
	}
	for k, v := range secret.Data {
		out[k] = string(v)
	}
	return out
}

func TestCreateS3StoresKeysInASecretOwnedByTheProfile(t *testing.T) {
	w := newWorld()
	view := mustCreate(t, w.svc, s3Input(" MinIO "))

	if view.Name != "MinIO" || view.Type != TypeS3 || view.S3 == nil || !view.S3.CredentialsSet || view.Test.State != StatePending {
		t.Errorf("view = %+v, s3 = %+v", view, view.S3)
	}
	keys := secretOf(t, w, view.ID)
	if keys[SecretKeyAccess] != "AKIATEST" || keys[SecretKeySecret] != "very-secret-value" {
		t.Errorf("secret = %v", keys)
	}
	secret, _ := w.kube.CoreV1().Secrets(testNamespace).Get(context.Background(), SecretName(view.ID), metav1.GetOptions{})
	if len(secret.OwnerReferences) != 1 || secret.OwnerReferences[0].Name != view.ID || secret.OwnerReferences[0].Kind != Kind {
		t.Errorf("owner references = %+v; deleting the profile must delete the secret", secret.OwnerReferences)
	}

	stored, err := w.store.Get(context.Background(), view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Spec.S3.CredentialsSecret != SecretName(view.ID) || stored.Spec.TestRequestedAt == nil {
		t.Errorf("stored spec = %+v", stored.Spec)
	}
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), "AKIATEST") || strings.Contains(string(raw), "very-secret-value") {
		t.Errorf("the profile must not contain the keys: %s", raw)
	}
}

func TestViewNeverContainsTheKeys(t *testing.T) {
	w := newWorld()
	view := mustCreate(t, w.svc, s3Input("MinIO"))
	raw, _ := json.Marshal(view)
	for _, secret := range []string{"AKIATEST", "very-secret-value"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the view contains %q: %s", secret, raw)
		}
	}
}

func TestCreateNFSNeedsNoSecret(t *testing.T) {
	w := newWorld()
	view := mustCreate(t, w.svc, nfsInput("Share"))
	if view.Type != TypeNFS || view.NFS == nil || view.S3 != nil {
		t.Errorf("view = %+v", view)
	}
	if secrets, _ := w.kube.CoreV1().Secrets(testNamespace).List(context.Background(), metav1.ListOptions{}); len(secrets.Items) != 0 {
		t.Errorf("secrets = %d, want none", len(secrets.Items))
	}
}

func TestCreateRefuses(t *testing.T) {
	w := newWorld()
	mustCreate(t, w.svc, nfsInput("Share"))

	noKeys := s3Input("S3")
	noKeys.S3.AccessKey, noKeys.S3.SecretKey = "", ""
	tests := []struct {
		name string
		in   Input
		want error
	}{
		{"same name, other case", nfsInput("share"), ErrNameTaken},
		{"invalid", Input{Name: "x", Type: "ftp"}, ErrInvalid},
		{"s3 without keys", noKeys, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := w.svc.Create(context.Background(), tt.in); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCreateRollsBackWhenTheSecretCannotBeStored(t *testing.T) {
	w := newWorld()
	w.kube.PrependReactor("create", "secrets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})

	if _, err := w.svc.Create(context.Background(), s3Input("MinIO")); err == nil {
		t.Fatal("want an error")
	}
	if all, _ := w.store.List(context.Background(), ""); len(all) != 0 {
		t.Errorf("profiles = %d; a location without keys must not stay behind", len(all))
	}
}

func TestUpdateKeepsKeysWhenLeftOut(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, s3Input("MinIO"))

	in := s3Input("MinIO renamed")
	in.S3.AccessKey, in.S3.SecretKey = "", ""
	in.S3.Bucket = "other-bucket"
	updated, err := w.svc.Update(context.Background(), created.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "MinIO renamed" || updated.S3.Bucket != "other-bucket" || !updated.S3.CredentialsSet {
		t.Errorf("updated = %+v, s3 = %+v", updated, updated.S3)
	}
	if keys := secretOf(t, w, created.ID); keys[SecretKeySecret] != "very-secret-value" {
		t.Errorf("secret = %v, want the old keys", keys)
	}
}

func TestUpdateReplacesKeysWhenGiven(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, s3Input("MinIO"))

	in := s3Input("MinIO")
	in.S3.AccessKey, in.S3.SecretKey = "NEWKEY", "new-secret"
	if _, err := w.svc.Update(context.Background(), created.ID, in); err != nil {
		t.Fatal(err)
	}
	if keys := secretOf(t, w, created.ID); keys[SecretKeyAccess] != "NEWKEY" || keys[SecretKeySecret] != "new-secret" {
		t.Errorf("secret = %v", keys)
	}
}

func TestUpdateAsksForANewTest(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, nfsInput("Share"))
	stored, _ := w.store.Get(context.Background(), created.ID)
	stored.Status.LastTest = &TestResult{RequestedAt: stored.Spec.TestRequestedAt, At: metav1.NewTime(testNow), OK: true}
	if _, err := w.store.UpdateStatus(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.svc.Get(context.Background(), created.ID); got.Test.State != StateSucceeded {
		t.Fatalf("state = %q, want succeeded", got.Test.State)
	}

	later := testNow.Add(time.Minute)
	w.svc.now = func() time.Time { return later }
	updated, err := w.svc.Update(context.Background(), created.ID, nfsInput("Share"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Test.State != StatePending {
		t.Errorf("state = %q; changing a location must show the old result as outdated", updated.Test.State)
	}
}

func TestUpdateRefuses(t *testing.T) {
	w := newWorld()
	s3 := mustCreate(t, w.svc, s3Input("MinIO"))
	mustCreate(t, w.svc, nfsInput("Share"))

	if _, err := w.svc.Update(context.Background(), s3.ID, nfsInput("MinIO")); !errors.Is(err, ErrInvalid) {
		t.Errorf("changing the type: error = %v, want ErrInvalid", err)
	}
	if _, err := w.svc.Update(context.Background(), s3.ID, s3Input("share")); !errors.Is(err, ErrNameTaken) {
		t.Errorf("taking another name: error = %v, want ErrNameTaken", err)
	}
	if _, err := w.svc.Update(context.Background(), s3.ID, s3Input("MinIO")); err != nil {
		t.Errorf("keeping the own name must work: %v", err)
	}
	if _, err := w.svc.Update(context.Background(), "l-nope", s3Input("x")); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: error = %v, want ErrNotFound", err)
	}
}

func TestRequestTestMakesTheResultPendingAgain(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, nfsInput("Share"))
	stored, _ := w.store.Get(context.Background(), created.ID)
	stored.Status.LastTest = &TestResult{
		RequestedAt: stored.Spec.TestRequestedAt, At: metav1.NewTime(testNow), OK: false, ErrorCode: CodeNFSMountFailed,
		Checks: []snapshot.StepStatus{{ID: CheckMount, Phase: snapshot.PhaseFailed}},
	}
	if _, err := w.store.UpdateStatus(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	failed, _ := w.svc.Get(context.Background(), created.ID)
	if failed.Test.State != StateFailed || failed.Test.ErrorCode != CodeNFSMountFailed || len(failed.Test.Checks) != 1 {
		t.Fatalf("test = %+v", failed.Test)
	}

	w.svc.now = func() time.Time { return testNow.Add(time.Minute) }
	view, err := w.svc.RequestTest(context.Background(), created.ID)
	if err != nil || view.Test.State != StatePending {
		t.Errorf("RequestTest = %+v, %v", view.Test, err)
	}
	if _, err := w.svc.RequestTest(context.Background(), "l-nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: error = %v", err)
	}
}

func TestDelete(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, nfsInput("Share"))
	if err := w.svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.svc.Get(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete: %v", err)
	}
	if err := w.svc.Delete(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: error = %v, want ErrNotFound", err)
	}
}

func TestListNewestFirstWithPaging(t *testing.T) {
	w := newWorld()
	clock := testNow
	w.svc.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	a := mustCreate(t, w.svc, nfsInput("A"))
	b := mustCreate(t, w.svc, nfsInput("B"))
	c := mustCreate(t, w.svc, nfsInput("C"))

	first, err := w.svc.List(context.Background(), 2, "")
	if err != nil || first.Total != 3 || len(first.Items) != 2 || first.Items[0].ID != c.ID || first.Items[1].ID != b.ID || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	second, err := w.svc.List(context.Background(), 2, first.NextCursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != a.ID || second.NextCursor != "" {
		t.Fatalf("second page = %+v, %v", second, err)
	}
}

func TestDeleteHonoursGuards(t *testing.T) {
	w := newWorld()
	created := mustCreate(t, w.svc, nfsInput("Share"))
	w.svc.AddDeleteGuard(func(_ context.Context, id string) error {
		if id == created.ID {
			return fmt.Errorf("%w: used by a policy", ErrInUse)
		}
		return nil
	})

	if err := w.svc.Delete(context.Background(), created.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("error = %v, want ErrInUse", err)
	}
	if _, err := w.svc.Get(context.Background(), created.ID); err != nil {
		t.Errorf("a refused delete must keep the location: %v", err)
	}
}

type fakeCounter map[string]int

func (f fakeCounter) CountByLocation(context.Context) (map[string]int, error) { return f, nil }

func TestViewsCountTheStoredBackups(t *testing.T) {
	w := newWorld()
	a := mustCreate(t, w.svc, nfsInput("A"))
	b := mustCreate(t, w.svc, nfsInput("B"))
	w.svc.CountExports(fakeCounter{a.ID: 3})

	got, err := w.svc.Get(context.Background(), a.ID)
	if err != nil || got.Exports != 3 {
		t.Errorf("Get = %d exports, %v", got.Exports, err)
	}
	page, err := w.svc.List(context.Background(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range page.Items {
		counts[v.ID] = v.Exports
	}
	if counts[a.ID] != 3 || counts[b.ID] != 0 {
		t.Errorf("counts = %v", counts)
	}
}

func TestTwoLocationsCannotWriteToTheSamePlace(t *testing.T) {
	w := newWorld()
	first := mustCreate(t, w.svc, s3Input("First"))

	same := s3Input("Second")
	same.S3.Endpoint = "HTTP://minio.minio:9000/"
	same.S3.Prefix = "/"
	if _, err := w.svc.Create(context.Background(), same); !errors.Is(err, ErrTargetTaken) {
		t.Fatalf("same bucket: error = %v, want ErrTargetTaken", err)
	}

	other := s3Input("Second")
	other.S3.Prefix = "second"
	second := mustCreate(t, w.svc, other)
	clash := s3Input("Second")
	if _, err := w.svc.Update(context.Background(), second.ID, clash); !errors.Is(err, ErrTargetTaken) {
		t.Errorf("moving onto the other location: error = %v, want ErrTargetTaken", err)
	}
	if _, err := w.svc.Update(context.Background(), first.ID, s3Input("First again")); err != nil {
		t.Errorf("saving a location without changing its place: %v", err)
	}
}
