package location

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProbeDirectory is what the test pod of an NFS location runs inside the mounted share: it writes a file, reads it
// back and removes it. It returns the stage that failed ("write", "read" or "delete") together with the error.
func ProbeDirectory(dir string) (stage string, err error) {
	name := filepath.Join(dir, ".restora-test-"+probeName())
	payload := []byte("restora connection test")
	if err := os.WriteFile(name, payload, 0o600); err != nil {
		return CheckWrite, err
	}
	got, err := os.ReadFile(name)
	if err != nil {
		_ = os.Remove(name)
		return CheckRead, err
	}
	if !bytes.Equal(got, payload) {
		_ = os.Remove(name)
		return CheckRead, fmt.Errorf("the file that was read back differs from the one that was written")
	}
	if err := os.Remove(name); err != nil {
		return CheckDelete, err
	}
	return "", nil
}

func probeName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// probeReport is what the test pod leaves in its termination message for the controller.
type probeReport struct {
	Stage string `json:"stage,omitempty"`
	Error string `json:"error,omitempty"`
}

// RunProbe is the command of the test pod: it probes the directory, writes the report to terminationLog and returns
// the exit code of the process.
func RunProbe(dir, terminationLog string) int {
	stage, err := ProbeDirectory(dir)
	report := probeReport{Stage: stage}
	if err != nil {
		report.Error = err.Error()
	}
	if data, marshalErr := json.Marshal(report); marshalErr == nil {
		_ = os.WriteFile(terminationLog, data, 0o600)
	}
	if err != nil {
		return 1
	}
	return 0
}
