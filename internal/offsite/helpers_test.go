package offsite

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"

	"restora.local/restora/internal/location"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// call is one Kopia run.
type call struct {
	Args []string
	Env  []string
}

func (c call) has(words ...string) bool {
	return strings.Contains(" "+strings.Join(c.Args, " ")+" ", " "+strings.Join(words, " ")+" ")
}

// fakeRunner answers Kopia calls from a script: the first rule whose words all appear in the arguments wins.
type fakeRunner struct {
	mu    sync.Mutex
	calls []call
	rules []rule
}

type rule struct {
	words  []string
	stdout string
	stderr string
	err    error
	// once makes the rule apply a single time.
	once bool
	used bool
}

func (f *fakeRunner) on(stdout string, words ...string) *rule {
	r := &rule{words: words, stdout: stdout}
	f.rules = append(f.rules, *r)
	return &f.rules[len(f.rules)-1]
}

func (f *fakeRunner) fail(stderr string, words ...string) {
	f.rules = append(f.rules, rule{words: words, stderr: stderr, err: errExit})
}

func (f *fakeRunner) failOnce(stderr string, words ...string) {
	f.rules = append(f.rules, rule{words: words, stderr: stderr, err: errExit, once: true})
}

type exitError struct{}

func (exitError) Error() string { return "exit status 1" }

var errExit = exitError{}

func (f *fakeRunner) Run(_ context.Context, env []string, args ...string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{Args: args, Env: env})
	c := call{Args: args}
	for i := range f.rules {
		r := &f.rules[i]
		if r.once && r.used {
			continue
		}
		match := true
		for _, w := range r.words {
			if !c.has(w) {
				match = false
				break
			}
		}
		if match {
			r.used = true
			return r.stdout, r.stderr, r.err
		}
	}
	return "", "", nil
}

func (f *fakeRunner) ran(words ...string) int {
	n := 0
	for _, c := range f.calls {
		if c.has(words...) {
			n++
		}
	}
	return n
}

func s3Target() Target {
	return Target{
		Location: "l-1", Type: location.TypeS3,
		S3:        &location.S3Spec{Endpoint: "http://minio.minio:9000", Bucket: "restora", Prefix: "backups"},
		AccessKey: "AKIATEST", SecretKey: "very-secret-value", Password: "repo-password",
	}
}

func nfsTarget() Target {
	return Target{Location: "l-2", Type: location.TypeNFS, NFS: &location.NFSSpec{Server: "nfs", Path: "/exports"}, Password: "repo-password"}
}

const snapshotJSON = `Snapshotting restora@demo:/volumes/data ...
{"id":"abc123","source":{"host":"demo","userName":"restora","path":"/volumes/data"},"rootEntry":{"name":"data","type":"d","summ":{"size":3000006,"files":2,"symlinks":0,"dirs":1,"numFailed":0}},"tags":{"tag:volume":"data"}}
Running full maintenance...
GC found 0 unused contents (0 B)
`
