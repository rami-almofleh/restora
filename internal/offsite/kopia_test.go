package offsite

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func newKopia(r Runner, target Target) *Kopia {
	return &Kopia{Runner: r, Target: target, Dir: "/tmp/k", Host: "demo"}
}

func TestRepoArgsForS3(t *testing.T) {
	provider, args := s3Target().repoArgs()
	joined := strings.Join(args, " ")
	if provider != "s3" || !strings.Contains(joined, "--bucket restora") || !strings.Contains(joined, "--prefix backups/restora-repository/") ||
		!strings.Contains(joined, "--endpoint minio.minio:9000") || !strings.Contains(joined, "--disable-tls") || strings.Contains(joined, "verification") {
		t.Errorf("provider = %s, args = %v", provider, args)
	}

	secure := s3Target()
	secure.S3.Endpoint, secure.S3.Prefix, secure.S3.Region, secure.S3.SkipTLSVerify = "https://s3.example.com", "", "eu-central-1", true
	_, args = secure.repoArgs()
	joined = strings.Join(args, " ")
	if !strings.Contains(joined, "--prefix restora-repository/") || strings.Contains(joined, "--disable-tls ") || !strings.Contains(joined, "--disable-tls-verification") ||
		!strings.Contains(joined, "--region eu-central-1") {
		t.Errorf("args = %v", args)
	}
}

func TestRepoArgsForNFS(t *testing.T) {
	provider, args := nfsTarget().repoArgs()
	if provider != "filesystem" || !slices.Equal(args, []string{"--path", "/repo-nfs/restora-repository"}) {
		t.Errorf("provider = %s, args = %v", provider, args)
	}
}

func TestTargetEnvRoundTrip(t *testing.T) {
	for _, target := range []Target{s3Target(), nfsTarget()} {
		env := target.Env()
		back := TargetFromEnv(func(k string) string { return env[k] })
		provider, args := target.repoArgs()
		provider2, args2 := back.repoArgs()
		if provider != provider2 || !slices.Equal(args, args2) || back.Password != target.Password {
			t.Errorf("%s: after the round trip: %s %v, want %s %v", target.Type, provider2, args2, provider, args)
		}
	}
}

func TestConnectOpensAnExistingRepository(t *testing.T) {
	r := &fakeRunner{}
	if err := newKopia(r, s3Target()).Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.ran("repository", "connect", "s3") != 1 || r.ran("repository", "create") != 0 {
		t.Errorf("calls = %v", r.calls)
	}
	connect := r.calls[0]
	if !connect.has("--override-hostname=demo") || !connect.has("--override-username=restora") {
		t.Errorf("the snapshots need an identity per application: %v", connect.Args)
	}
	if !slices.Contains(connect.Env, "KOPIA_PASSWORD=repo-password") || !slices.Contains(connect.Env, "KOPIA_CONFIG_PATH=/tmp/k/repo.config") {
		t.Errorf("env = %v", connect.Env)
	}
}

func TestConnectCreatesTheRepositoryTheFirstTime(t *testing.T) {
	r := &fakeRunner{}
	r.fail("ERROR error connecting to repository: repository not initialized in the provided storage", "repository", "connect")
	if err := newKopia(r, s3Target()).Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.ran("repository", "create", "s3") != 1 {
		t.Errorf("calls = %v", r.calls)
	}
}

func TestReadingNeverCreatesTheRepository(t *testing.T) {
	r := &fakeRunner{}
	r.fail("ERROR error connecting to repository: repository not initialized in the provided storage", "repository", "connect")
	err := newKopia(r, s3Target()).ConnectExisting(context.Background())
	if !errors.Is(err, ErrNoRepository) || r.ran("repository", "create") != 0 {
		t.Errorf("error = %v, create calls = %d; a restore must not write to the location", err, r.ran("repository", "create"))
	}
	if Classify(err) != CodeNoRepository {
		t.Errorf("code = %s, want %s", Classify(err), CodeNoRepository)
	}
}

func TestConnectSurvivesAnotherMoverCreatingTheRepositoryFirst(t *testing.T) {
	r := &fakeRunner{}
	r.failOnce("repository not initialized in the provided storage", "repository", "connect")
	r.fail("repository already initialized", "repository", "create")
	if err := newKopia(r, s3Target()).Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v; a lost race must end in a connection", err)
	}
}

func TestConnectRecognisesAWrongPassword(t *testing.T) {
	r := &fakeRunner{}
	r.fail("ERROR error connecting to repository: invalid repository password", "repository", "connect")
	err := newKopia(r, s3Target()).Connect(context.Background())
	if !errors.Is(err, ErrWrongPassword) || r.ran("repository", "create") != 0 {
		t.Errorf("error = %v, create calls = %d; a wrong password must never create a second repository", err, r.ran("repository", "create"))
	}
	if Classify(err) != CodeWrongPassword {
		t.Errorf("code = %s", Classify(err))
	}
}

func TestErrorsNeverContainTheKeys(t *testing.T) {
	r := &fakeRunner{}
	r.fail("dial tcp: lookup minio: no such host", "repository", "connect")
	err := newKopia(r, s3Target()).Connect(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	for _, secret := range []string{"very-secret-value", "AKIATEST", "repo-password"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the error contains %q: %v", secret, err)
		}
	}
	if Classify(err) != CodeEndpointUnreachable {
		t.Errorf("code = %s", Classify(err))
	}
}

func TestSnapshotParsesTheResult(t *testing.T) {
	r := &fakeRunner{}
	r.on(snapshotJSON, "snapshot", "create")
	got, err := newKopia(r, s3Target()).Snapshot(context.Background(), "/volumes/data", map[string]string{"restora-volume": "data", "restora-snapshot": "s-1"}, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "abc123" || got.SizeBytes != 3000006 || got.FileCount != 2 || got.Failed != 0 {
		t.Errorf("result = %+v", got)
	}
	c := r.calls[0]
	if !c.has("snapshot", "create", "/volumes/data") || !c.has("--tags=restora-snapshot:s-1") || !c.has("--tags=restora-volume:data") {
		t.Errorf("args = %v", c.Args)
	}
}

func TestSnapshotWithoutResultIsAnError(t *testing.T) {
	r := &fakeRunner{}
	r.on("Snapshotting ...\n", "snapshot", "create")
	if _, err := newKopia(r, s3Target()).Snapshot(context.Background(), "/x", nil, ""); err == nil {
		t.Error("an answer without the snapshot must be an error")
	}
}

func TestRepositoryBytes(t *testing.T) {
	r := &fakeRunner{}
	r.on("Count: 13\nTotal: 3024074\nAverage: 232621\n", "blob", "stats")
	n, err := newKopia(r, s3Target()).RepositoryBytes(context.Background())
	if err != nil || n != 3024074 {
		t.Errorf("RepositoryBytes = %d, %v", n, err)
	}
	r2 := &fakeRunner{}
	r2.on("nothing useful", "blob", "stats")
	if _, err := newKopia(r2, s3Target()).RepositoryBytes(context.Background()); err == nil {
		t.Error("an answer without a total must be an error")
	}
}

func TestFindPicksTheNewestSnapshot(t *testing.T) {
	r := &fakeRunner{}
	r.on(`[{"id":"old","endTime":"2026-11-20T10:00:00Z"},{"id":"new","endTime":"2026-11-20T11:00:00Z"}]`, "snapshot", "list")
	id, err := newKopia(r, s3Target()).Find(context.Background(), map[string]string{"restora-snapshot": "s-1"})
	if err != nil || id != "new" {
		t.Errorf("Find = %q, %v", id, err)
	}

	r2 := &fakeRunner{}
	r2.on(`[]`, "snapshot", "list")
	if _, err := newKopia(r2, s3Target()).Find(context.Background(), nil); !errors.Is(err, ErrNoSnapshot) {
		t.Errorf("error = %v, want ErrNoSnapshot", err)
	}
}

func TestRestore(t *testing.T) {
	r := &fakeRunner{}
	if err := newKopia(r, s3Target()).Restore(context.Background(), "abc123", "/volumes/data"); err != nil {
		t.Fatal(err)
	}
	if !r.calls[0].has("restore", "abc123", "/volumes/data") {
		t.Errorf("args = %v", r.calls[0].Args)
	}
}

func TestParseAndFormatTags(t *testing.T) {
	tags := map[string]string{"restora-snapshot": "s-1", "restora-volume": "data"}
	if got := parseTags(formatTags(tags)); !slices.Equal(tagArgs(got), tagArgs(tags)) {
		t.Errorf("round trip = %v", got)
	}
	if got := parseTags(" a=b , broken, =x, c=d=e "); len(got) != 2 || got["a"] != "b" || got["c"] != "d=e" {
		t.Errorf("parseTags = %v", got)
	}
}
