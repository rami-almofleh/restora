package offsite

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MoverCommand is the argument that makes the controller binary act as a data mover instead of as the controller.
const MoverCommand = "mover"

// Environment variables that tell the data mover what to do. The repository is described by the RESTORA_REPO_* ones.
const (
	envMoverDir         = "RESTORA_MOVER_DIR"
	envMoverHost        = "RESTORA_MOVER_HOST"
	envMoverTags        = "RESTORA_MOVER_TAGS"
	envMoverSnapshot    = "RESTORA_MOVER_SNAPSHOT"
	envMoverDescription = "RESTORA_MOVER_DESCRIPTION"
)

// Stdout is where a data mover prints the saved resources of a backup it fetched. The controller reads them from the
// log of the pod.
var Stdout io.Writer = os.Stdout

// Markers around a file in the output of "fetch-bundle". The content is base64, in one line.
const (
	bundleBegin = "RESTORA-FILE-BEGIN"
	bundleEnd   = "RESTORA-FILE-END"
	// BundleDir is where "fetch-bundle" restores the saved resources inside the pod.
	BundleDir = "/tmp/restora-bundle"
)

// moverWorkDir is where the data mover keeps the Kopia configuration and cache. It is gone with the pod.
const moverWorkDir = "/tmp/restora-kopia"

// maxReportBytes keeps the report below the limit for termination messages (4096 bytes).
const maxReportBytes = 3500

// Failure codes a data mover reports. The controller shows them to the user, who sees plain sentences for them.
const (
	CodeWrongPassword       = "repository_password_invalid"
	CodeAccessDenied        = "access_denied"
	CodeBucketNotFound      = "bucket_not_found"
	CodeEndpointUnreachable = "endpoint_unreachable"
	CodeNotWritable         = "nfs_not_writable"
	CodeNotInRepository     = "snapshot_not_in_repository"
	CodeNoRepository        = "repository_missing"
	CodeFailed              = "export_failed"
)

// MoverReport is what a data mover leaves in its termination message for the controller.
type MoverReport struct {
	SnapshotID string `json:"snapshotId,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	FileCount  int64  `json:"fileCount,omitempty"`
	AddedBytes int64  `json:"addedBytes,omitempty"`
	Code       string `json:"code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// RunMover is the whole program of a data mover pod. args[0] is "backup" or "restore". It writes a MoverReport to
// terminationLog and returns the exit code.
func RunMover(ctx context.Context, args []string, getenv func(string) string, runner Runner, terminationLog string) int {
	report := runMover(ctx, args, getenv, runner)
	if data, err := json.Marshal(report); err == nil {
		if len(data) > maxReportBytes {
			report.Error = tail(report.Error, 500)
			data, _ = json.Marshal(report)
		}
		_ = os.WriteFile(terminationLog, data, 0o600)
	}
	if report.Error != "" {
		return 1
	}
	return 0
}

func runMover(ctx context.Context, args []string, getenv func(string) string, runner Runner) MoverReport {
	if len(args) != 1 || (args[0] != "backup" && args[0] != "restore" && args[0] != "fetch-bundle") {
		return MoverReport{Code: CodeFailed, Error: "usage: mover backup|restore|fetch-bundle"}
	}
	dir := getenv(envMoverDir)
	if args[0] == "fetch-bundle" {
		dir = BundleDir
	}
	if dir == "" {
		return MoverReport{Code: CodeFailed, Error: envMoverDir + " is not set"}
	}
	k := &Kopia{Runner: runner, Target: TargetFromEnv(getenv), Dir: moverWorkDir, Host: getenv(envMoverHost)}
	if err := os.MkdirAll(moverWorkDir, 0o700); err != nil {
		return failed(err)
	}
	connect := k.Connect
	if args[0] != "backup" {
		connect = k.ConnectExisting
	}
	if err := connect(ctx); err != nil {
		return failed(err)
	}

	if args[0] == "restore" || args[0] == "fetch-bundle" {
		id := getenv(envMoverSnapshot)
		if id == "" {
			return MoverReport{Code: CodeFailed, Error: envMoverSnapshot + " is not set"}
		}
		if args[0] == "fetch-bundle" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return failed(err)
			}
		}
		if err := k.Restore(ctx, id, dir); err != nil {
			return failed(err)
		}
		if args[0] == "fetch-bundle" {
			if err := printFiles(dir); err != nil {
				return failed(err)
			}
		}
		return MoverReport{SnapshotID: id}
	}

	before, err := k.RepositoryBytes(ctx)
	if err != nil {
		return failed(err)
	}
	result, err := k.Snapshot(ctx, dir, parseTags(getenv(envMoverTags)), getenv(envMoverDescription))
	if err != nil {
		return failed(err)
	}
	if result.Failed > 0 {
		return MoverReport{Code: CodeFailed, Error: fmt.Sprintf("%d files could not be read", result.Failed)}
	}
	after, err := k.RepositoryBytes(ctx)
	if err != nil {
		return failed(err)
	}
	return MoverReport{SnapshotID: result.ID, SizeBytes: result.SizeBytes, FileCount: result.FileCount, AddedBytes: max(after-before, 0)}
}

func failed(err error) MoverReport {
	return MoverReport{Code: Classify(err), Error: err.Error()}
}

// Classify turns an error of Kopia into one of the failure codes.
func Classify(err error) string {
	if errors.Is(err, ErrWrongPassword) {
		return CodeWrongPassword
	}
	if errors.Is(err, ErrNoRepository) {
		return CodeNoRepository
	}
	if errors.Is(err, ErrNoSnapshot) {
		return CodeNotInRepository
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "accessdenied"), strings.Contains(text, "access denied"), strings.Contains(text, "invalidaccesskeyid"), strings.Contains(text, "signaturedoesnotmatch"):
		return CodeAccessDenied
	case strings.Contains(text, "nosuchbucket"), strings.Contains(text, "bucket does not exist"):
		return CodeBucketNotFound
	case strings.Contains(text, "no such host"), strings.Contains(text, "connection refused"), strings.Contains(text, "i/o timeout"), strings.Contains(text, "dial tcp"):
		return CodeEndpointUnreachable
	case strings.Contains(text, "permission denied"), strings.Contains(text, "read-only file system"):
		return CodeNotWritable
	}
	return CodeFailed
}

// parseTags reads "key=value,key=value".
func parseTags(s string) map[string]string {
	tags := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && k != "" {
			tags[k] = v
		}
	}
	return tags
}

// formatTags is the reverse of parseTags.
func formatTags(tags map[string]string) string {
	parts := make([]string, 0, len(tags))
	for _, a := range tagArgs(tags) {
		kv := strings.TrimPrefix(a, "--tags=")
		k, v, _ := strings.Cut(kv, ":")
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

// printFiles prints the files of a directory in the form ParseFiles reads: a marker line with the name, the content
// as base64 in one line, an end marker.
func printFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		// A secret mounted as a volume shows up as links: the files point into a hidden directory. The files are what
		// counts, so follow the links and skip the hidden entries and directories.
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(Stdout, "%s %s\n%s\n%s\n", bundleBegin, e.Name(), base64.StdEncoding.EncodeToString(data), bundleEnd)
	}
	return nil
}

// ParseFiles reads the output of "fetch-bundle" and returns the files by name.
func ParseFiles(output string) (map[string][]byte, error) {
	files := map[string][]byte{}
	lines := strings.Split(output, "\n")
	for i := 0; i < len(lines); i++ {
		name, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), bundleBegin+" ")
		if !ok {
			continue
		}
		if i+2 >= len(lines) || strings.TrimSpace(lines[i+2]) != bundleEnd {
			return nil, errors.New("the output of the data mover is cut off")
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[i+1]))
		if err != nil {
			return nil, fmt.Errorf("read %s from the data mover: %w", name, err)
		}
		files[name] = data
		i += 2
	}
	if len(files) == 0 {
		return nil, errors.New("the data mover printed no files")
	}
	return files, nil
}
