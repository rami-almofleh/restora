package offsite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Kopia is the command that stores and restores data. The image of the data mover has it.
const kopiaBinary = "kopia"

// Runner runs the Kopia command. Tests replace it.
type Runner interface {
	// Run runs Kopia with the arguments and the extra environment and returns what it wrote.
	Run(ctx context.Context, env []string, args ...string) (stdout, stderr string, err error)
}

// ExecRunner runs the real Kopia binary.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, env []string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, kopiaBinary, args...)
	cmd.Env = append(cmd.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Errors of the repository.
var (
	// ErrWrongPassword is returned when the repository exists and the password does not open it.
	ErrWrongPassword = errors.New("the repository password is wrong")
	// ErrNoRepository is returned when a location holds no repository although one was expected: it was emptied or
	// replaced. Restoring never creates one.
	ErrNoRepository = errors.New("the storage location holds no repository")
	// ErrNoSnapshot is returned when the repository holds no snapshot with the asked tags.
	ErrNoSnapshot = errors.New("no such snapshot in the repository")
)

// CommandError is a failed Kopia call. It carries the end of what Kopia wrote, never the arguments, which hold keys.
type CommandError struct {
	Action string
	Output string
	Err    error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("kopia %s: %v: %s", e.Action, e.Err, e.Output)
}

func (e *CommandError) Unwrap() error { return e.Err }

// tail keeps the last n bytes of s: errors are at the end.
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// Kopia works with the repository of one target.
type Kopia struct {
	Runner Runner
	Target Target
	// Dir holds the Kopia configuration, cache and logs of this repository connection.
	Dir string
	// Host is the identity under which snapshots are stored. Restora uses the application, so every application has
	// its own history and Kopia compares a new snapshot with the previous one of the same application.
	Host string
}

func (k *Kopia) env() []string {
	return []string{
		"KOPIA_PASSWORD=" + k.Target.Password,
		"KOPIA_CONFIG_PATH=" + filepath.Join(k.Dir, "repo.config"),
		"KOPIA_CACHE_DIRECTORY=" + filepath.Join(k.Dir, "cache"),
		"KOPIA_LOG_DIR=" + filepath.Join(k.Dir, "logs"),
		"KOPIA_CHECK_FOR_UPDATES=false",
		"HOME=" + k.Dir,
	}
}

func (k *Kopia) run(ctx context.Context, action string, args ...string) (string, error) {
	stdout, stderr, err := k.Runner.Run(ctx, k.env(), args...)
	if err != nil {
		return stdout, &CommandError{Action: action, Output: tail(stderr+"\n"+stdout, 1200), Err: err}
	}
	return stdout, nil
}

// Connect opens the repository and creates it first when the location has none yet.
func (k *Kopia) Connect(ctx context.Context) error {
	return k.connect(ctx, true)
}

// ConnectExisting opens the repository and fails with ErrNoRepository when the location has none. Reading from a
// location must never write to it.
func (k *Kopia) ConnectExisting(ctx context.Context) error {
	return k.connect(ctx, false)
}

func (k *Kopia) connect(ctx context.Context, create bool) error {
	provider, args := k.Target.repoArgs()
	identity := []string{"--override-username=restora", "--override-hostname=" + k.Host}
	connect := append(append([]string{"repository", "connect", provider}, args...), identity...)
	_, err := k.run(ctx, "connect", connect...)
	if err == nil {
		return nil
	}
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		return err
	}
	out := strings.ToLower(cmdErr.Output)
	switch {
	case strings.Contains(out, "invalid repository password"):
		return fmt.Errorf("%w: %w", ErrWrongPassword, err)
	case strings.Contains(out, "repository not initialized") && !create:
		return fmt.Errorf("%w: %w", ErrNoRepository, err)
	case strings.Contains(out, "repository not initialized"):
		create := append(append([]string{"repository", "create", provider}, args...), identity...)
		if _, createErr := k.run(ctx, "create", create...); createErr != nil {
			// Another mover may have created it a moment ago.
			if _, retryErr := k.run(ctx, "connect", connect...); retryErr != nil {
				return createErr
			}
		}
		return nil
	default:
		return err
	}
}

// SnapshotResult is what Kopia reports about a snapshot it created.
type SnapshotResult struct {
	ID        string
	SizeBytes int64
	FileCount int64
	// Failed is the number of files Kopia could not read.
	Failed int64
}

// Snapshot stores a directory in the repository under the given tags.
func (k *Kopia) Snapshot(ctx context.Context, dir string, tags map[string]string, description string) (SnapshotResult, error) {
	args := []string{"snapshot", "create", dir, "--json", "--description", description}
	args = append(args, tagArgs(tags)...)
	stdout, err := k.run(ctx, "snapshot create", args...)
	if err != nil {
		return SnapshotResult{}, err
	}
	return parseSnapshotResult(stdout)
}

// parseSnapshotResult finds the JSON line in what "snapshot create --json" wrote. Kopia adds progress lines before
// it and maintenance messages after it.
func parseSnapshotResult(stdout string) (SnapshotResult, error) {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var m struct {
			ID        string `json:"id"`
			RootEntry struct {
				Summary struct {
					Size      int64 `json:"size"`
					Files     int64 `json:"files"`
					NumFailed int64 `json:"numFailed"`
				} `json:"summ"`
			} `json:"rootEntry"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil || m.ID == "" {
			continue
		}
		return SnapshotResult{
			ID: m.ID, SizeBytes: m.RootEntry.Summary.Size, FileCount: m.RootEntry.Summary.Files, Failed: m.RootEntry.Summary.NumFailed,
		}, nil
	}
	return SnapshotResult{}, errors.New("kopia did not report the snapshot")
}

// RepositoryBytes returns how much the repository holds. The difference before and after a snapshot is what the
// snapshot added, which is little when most of the data was stored before.
func (k *Kopia) RepositoryBytes(ctx context.Context) (int64, error) {
	stdout, err := k.run(ctx, "blob stats", "blob", "stats", "--raw")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(stdout, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Total:"); ok {
			n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("read repository size %q: %w", rest, err)
			}
			return n, nil
		}
	}
	return 0, errors.New("kopia did not report the repository size")
}

// Restore writes a snapshot into a directory.
func (k *Kopia) Restore(ctx context.Context, snapshotID, dir string) error {
	_, err := k.run(ctx, "restore", "restore", snapshotID, dir)
	return err
}

// Find returns the id of the newest snapshot that carries all the tags.
func (k *Kopia) Find(ctx context.Context, tags map[string]string) (string, error) {
	args := []string{"snapshot", "list", "--all", "--json"}
	args = append(args, tagArgs(tags)...)
	stdout, err := k.run(ctx, "snapshot list", args...)
	if err != nil {
		return "", err
	}
	return newestSnapshot(stdout)
}

// newestSnapshot picks the latest snapshot from the output of "snapshot list --json".
func newestSnapshot(stdout string) (string, error) {
	var list []struct {
		ID      string `json:"id"`
		EndTime string `json:"endTime"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		return "", fmt.Errorf("read the snapshot list: %w", err)
	}
	if len(list) == 0 {
		return "", ErrNoSnapshot
	}
	// RFC 3339 times in UTC sort as text.
	sort.SliceStable(list, func(i, j int) bool { return list[i].EndTime > list[j].EndTime })
	return list[0].ID, nil
}

func tagArgs(tags map[string]string) []string {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys))
	for _, k := range keys {
		args = append(args, "--tags="+k+":"+tags[k])
	}
	return args
}
