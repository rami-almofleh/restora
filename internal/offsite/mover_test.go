package offsite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envOf(extra map[string]string, t Target) func(string) string {
	env := t.Env()
	for k, v := range extra {
		env[k] = v
	}
	return func(k string) string { return env[k] }
}

func readReport(t *testing.T, path string) MoverReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report MoverReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("report %q: %v", data, err)
	}
	return report
}

func TestMoverBackupReportsWhatWasAdded(t *testing.T) {
	r := &fakeRunner{}
	r.on(snapshotJSON, "snapshot", "create")
	sizes := []string{"Total: 1000\n", "Total: 1500\n"}
	r.rules = append(r.rules, rule{words: []string{"blob", "stats"}, stdout: sizes[0], once: true}, rule{words: []string{"blob", "stats"}, stdout: sizes[1]})
	log := filepath.Join(t.TempDir(), "termination-log")

	code := RunMover(context.Background(), []string{"backup"}, envOf(map[string]string{
		envMoverDir: "/volumes/data", envMoverHost: "demo", envMoverTags: "restora-snapshot=s-1,restora-volume=data", envMoverDescription: "demo/data",
	}, s3Target()), r, log)

	report := readReport(t, log)
	if code != 0 || report.Error != "" || report.SnapshotID != "abc123" || report.SizeBytes != 3000006 || report.FileCount != 2 || report.AddedBytes != 500 {
		t.Errorf("code %d, report %+v", code, report)
	}
}

func TestMoverRestore(t *testing.T) {
	r := &fakeRunner{}
	log := filepath.Join(t.TempDir(), "termination-log")

	code := RunMover(context.Background(), []string{"restore"}, envOf(map[string]string{
		envMoverDir: "/volumes/data", envMoverHost: "demo", envMoverSnapshot: "abc123",
	}, nfsTarget()), r, log)

	if code != 0 || r.ran("restore", "abc123", "/volumes/data") != 1 || readReport(t, log).SnapshotID != "abc123" {
		t.Errorf("code %d, calls %v", code, r.calls)
	}
}

func TestMoverReportsFailuresWithACode(t *testing.T) {
	tests := []struct {
		name     string
		stderr   string
		wantCode string
	}{
		{"wrong password", "invalid repository password", CodeWrongPassword},
		{"bucket missing", "NoSuchBucket: The specified bucket does not exist", CodeBucketNotFound},
		{"no rights", "AccessDenied: Access Denied.", CodeAccessDenied},
		{"unreachable", "dial tcp 10.0.0.1:9000: i/o timeout", CodeEndpointUnreachable},
		{"read-only share", "open /repo-nfs/x: permission denied", CodeNotWritable},
		{"something else", "boom", CodeFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{}
			r.fail(tt.stderr, "repository", "connect")
			log := filepath.Join(t.TempDir(), "termination-log")

			code := RunMover(context.Background(), []string{"backup"}, envOf(map[string]string{envMoverDir: "/x", envMoverHost: "demo"}, s3Target()), r, log)

			report := readReport(t, log)
			if code == 0 || report.Code != tt.wantCode || report.Error == "" {
				t.Errorf("code %d, report %+v", code, report)
			}
		})
	}
}

func TestMoverRefusesAFailingFile(t *testing.T) {
	r := &fakeRunner{}
	r.on(`{"id":"abc","rootEntry":{"summ":{"size":10,"files":3,"numFailed":2}}}`, "snapshot", "create")
	r.on("Total: 1\n", "blob", "stats")
	log := filepath.Join(t.TempDir(), "termination-log")

	code := RunMover(context.Background(), []string{"backup"}, envOf(map[string]string{envMoverDir: "/x", envMoverHost: "demo"}, s3Target()), r, log)
	if code == 0 || readReport(t, log).Error == "" {
		t.Error("a backup that could not read all files must not count as a success")
	}
}

func TestMoverNeedsItsArguments(t *testing.T) {
	log := filepath.Join(t.TempDir(), "termination-log")
	for name, args := range map[string][]string{"no command": nil, "unknown command": {"sing"}} {
		if code := RunMover(context.Background(), args, envOf(nil, s3Target()), &fakeRunner{}, log); code == 0 {
			t.Errorf("%s: exit code 0", name)
		}
	}
	if code := RunMover(context.Background(), []string{"backup"}, envOf(nil, s3Target()), &fakeRunner{}, log); code == 0 {
		t.Error("without a directory there is nothing to back up")
	}
	if code := RunMover(context.Background(), []string{"restore"}, envOf(map[string]string{envMoverDir: "/x"}, s3Target()), &fakeRunner{}, log); code == 0 {
		t.Error("a restore needs the id of a snapshot")
	}
}

func TestMoverReportFitsTheTerminationMessage(t *testing.T) {
	r := &fakeRunner{}
	long := ""
	for len(long) < 8000 {
		long += "an error line that goes on and on. "
	}
	r.fail(long, "repository", "connect")
	log := filepath.Join(t.TempDir(), "termination-log")
	RunMover(context.Background(), []string{"backup"}, envOf(map[string]string{envMoverDir: "/x"}, s3Target()), r, log)

	data, _ := os.ReadFile(log)
	if len(data) > 4096 {
		t.Errorf("report is %d bytes, the limit of a termination message is 4096", len(data))
	}
}

func TestFetchBundlePrintsTheFilesForTheController(t *testing.T) {
	// Kopia would restore into BundleDir. The test puts the files there itself.
	if err := os.MkdirAll(BundleDir, 0o700); err != nil {
		t.Skipf("cannot create %s: %v", BundleDir, err)
	}
	t.Cleanup(func() { os.RemoveAll(BundleDir) })
	if err := os.WriteFile(filepath.Join(BundleDir, "resources.json.gz"), []byte("gz-bytes\x00\x01"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(BundleDir, "manifest.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	old := Stdout
	Stdout = &out
	t.Cleanup(func() { Stdout = old })

	r := &fakeRunner{}
	log := filepath.Join(t.TempDir(), "termination-log")
	code := RunMover(context.Background(), []string{"fetch-bundle"}, envOf(map[string]string{envMoverHost: "demo", envMoverSnapshot: "res-1"}, s3Target()), r, log)

	if code != 0 || r.ran("restore", "res-1", BundleDir) != 1 {
		t.Fatalf("code %d, calls %v", code, r.calls)
	}
	files, err := ParseFiles(out.String())
	if err != nil {
		t.Fatal(err)
	}
	if string(files["resources.json.gz"]) != "gz-bytes\x00\x01" || string(files["manifest.json"]) != `{"version":1}` {
		t.Errorf("files = %q", files)
	}
}

func TestParseFilesRefusesBrokenOutput(t *testing.T) {
	tests := map[string]string{
		"nothing":    "some log line\n",
		"cut off":    bundleBegin + " a.txt\nYWJj\n",
		"bad base64": bundleBegin + " a.txt\n!!!\n" + bundleEnd + "\n",
	}
	for name, output := range tests {
		if _, err := ParseFiles(output); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	files, err := ParseFiles("log\n" + bundleBegin + " a.txt\nYWJj\n" + bundleEnd + "\nmore log\n")
	if err != nil || string(files["a.txt"]) != "abc" {
		t.Errorf("files = %q, %v; log lines around the files must be ignored", files, err)
	}
}

func TestFetchBundleFollowsTheLinksOfAMountedSecret(t *testing.T) {
	// A secret volume looks like this: the files are links into a hidden, timestamped directory.
	root := filepath.Join(t.TempDir(), "bundle")
	versioned := filepath.Join(root, "..2026_11_20_10_00_00.123")
	if err := os.MkdirAll(versioned, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versioned, "resources.json.gz"), []byte("gz-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..2026_11_20_10_00_00.123", filepath.Join(root, "..data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..data/resources.json.gz", filepath.Join(root, "resources.json.gz")); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	old := Stdout
	Stdout = &out
	t.Cleanup(func() { Stdout = old })
	if err := printFiles(root); err != nil {
		t.Fatalf("printFiles: %v", err)
	}
	files, err := ParseFiles(out.String())
	if err != nil || len(files) != 1 || string(files["resources.json.gz"]) != "gz-bytes" {
		t.Errorf("files = %q, %v; hidden entries must be skipped and links followed", files, err)
	}
}
