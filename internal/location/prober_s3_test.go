package location

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"restora.local/restora/internal/snapshot"
)

// fakeS3 is just enough of an S3 server for the test: a listing, and put, get and delete of objects. Any key signs in
// unless mode says otherwise.
type fakeS3 struct {
	mu      sync.Mutex
	mode    string // "", "badkey", "denied", "nobucket", "slow", "corrupt", "nodelete"
	objects map[string][]byte
	puts    int
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fail := func(status int, code string) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(status)
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, code)
	}
	switch f.mode {
	case "badkey":
		fail(http.StatusForbidden, "InvalidAccessKeyId")
		return
	case "denied":
		fail(http.StatusForbidden, "AccessDenied")
		return
	case "nobucket":
		fail(http.StatusNotFound, "NoSuchBucket")
		return
	case "slow":
		time.Sleep(300 * time.Millisecond)
	}

	// Path style: /bucket/key
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}
	switch {
	case r.Method == http.MethodGet && key == "":
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(struct {
			XMLName xml.Name `xml:"ListBucketResult"`
			Name    string
		}{Name: parts[0]})
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		if strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
			body = decodeAWSChunked(body)
		}
		f.objects[key] = body
		f.puts++
		w.Header().Set("ETag", `"abc"`)
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet:
		body, ok := f.objects[key]
		if !ok {
			fail(http.StatusNotFound, "NoSuchKey")
			return
		}
		if f.mode == "corrupt" {
			body = []byte("something else")
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		_, _ = w.Write(body)
	case r.Method == http.MethodDelete:
		if f.mode == "nodelete" {
			fail(http.StatusForbidden, "AccessDenied")
			return
		}
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// decodeAWSChunked unpacks the body of a signed streaming upload: size;chunk-signature=...\r\n data \r\n, ending with
// a chunk of size 0. Over plain HTTP the client sends uploads this way.
func decodeAWSChunked(body []byte) []byte {
	var out []byte
	for len(body) > 0 {
		end := bytes.Index(body, []byte("\r\n"))
		if end < 0 {
			break
		}
		size, err := strconv.ParseInt(strings.SplitN(string(body[:end]), ";", 2)[0], 16, 64)
		if err != nil || size == 0 {
			break
		}
		body = body[end+2:]
		out = append(out, body[:size]...)
		body = body[size+2:]
	}
	return out
}

type fakeKeys struct{ err error }

func (k fakeKeys) Credentials(context.Context, string) (string, string, error) {
	if k.err != nil {
		return "", "", k.err
	}
	return "AKIATEST", "very-secret-value", nil
}

func s3Profile(endpoint string) *LocationProfile {
	return &LocationProfile{Spec: Spec{
		Name: "Test", Type: TypeS3,
		S3: &S3Spec{Endpoint: endpoint, Bucket: "restora", Prefix: "backups", PathStyle: true, CredentialsSecret: "restora-location-l-1"},
	}}
}

func checkPhases(out *Outcome) string {
	var parts []string
	for _, c := range out.Checks {
		parts = append(parts, fmt.Sprintf("%s=%s", c.ID, c.Phase))
	}
	return strings.Join(parts, " ")
}

func TestS3ProbeSucceedsAndLeavesNothingBehind(t *testing.T) {
	fake := &fakeS3{objects: map[string][]byte{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	out := (&S3Prober{Keys: fakeKeys{}}).Probe(context.Background(), s3Profile(srv.URL))

	if !out.OK() {
		t.Fatalf("outcome = %+v", out)
	}
	if got := checkPhases(out); got != "connect=Succeeded write=Succeeded read=Succeeded delete=Succeeded" {
		t.Errorf("checks = %s", got)
	}
	if fake.puts != 1 || len(fake.objects) != 0 {
		t.Errorf("puts = %d, objects left = %d; the test object must be removed", fake.puts, len(fake.objects))
	}
}

func TestS3ProbeExplainsWhatWentWrong(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantCode  string
		wantState string
	}{
		{"wrong keys", "badkey", CodeInvalidCredentials, "connect=Failed write=Pending read=Pending delete=Pending"},
		{"no rights", "denied", CodeAccessDenied, "connect=Failed write=Pending read=Pending delete=Pending"},
		{"bucket missing", "nobucket", CodeBucketNotFound, "connect=Failed write=Pending read=Pending delete=Pending"},
		{"data comes back changed", "corrupt", CodeTestFailed, "connect=Succeeded write=Succeeded read=Failed delete=Pending"},
		{"object cannot be deleted", "nodelete", CodeAccessDenied, "connect=Succeeded write=Succeeded read=Succeeded delete=Failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeS3{mode: tt.mode, objects: map[string][]byte{}}
			srv := httptest.NewServer(fake)
			defer srv.Close()

			out := (&S3Prober{Keys: fakeKeys{}}).Probe(context.Background(), s3Profile(srv.URL))

			if out.Code != tt.wantCode || out.Message == "" || out.Hint == "" {
				t.Errorf("outcome = %+v, want code %q with message and hint", out, tt.wantCode)
			}
			if got := checkPhases(out); got != tt.wantState {
				t.Errorf("checks = %s, want %s", got, tt.wantState)
			}
			if strings.Contains(out.Message, "very-secret-value") {
				t.Error("the message must not contain the secret key")
			}
		})
	}
}

func TestS3ProbeEndpointUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens there any more

	out := (&S3Prober{Keys: fakeKeys{}, Timeout: 5 * time.Second}).Probe(context.Background(), s3Profile(url))
	if out.Code != CodeEndpointUnreachable {
		t.Errorf("outcome = %+v, want %s", out, CodeEndpointUnreachable)
	}
}

func TestS3ProbeTimesOut(t *testing.T) {
	fake := &fakeS3{mode: "slow", objects: map[string][]byte{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	out := (&S3Prober{Keys: fakeKeys{}, Timeout: 50 * time.Millisecond}).Probe(context.Background(), s3Profile(srv.URL))
	if out.Code != CodeTimeout && out.Code != CodeEndpointUnreachable {
		t.Errorf("outcome = %+v, want a timeout", out)
	}
	if out.OK() {
		t.Error("a timeout is no success")
	}
}

func TestS3ProbeTLS(t *testing.T) {
	fake := &fakeS3{objects: map[string][]byte{}}
	srv := httptest.NewTLSServer(fake)
	defer srv.Close()

	out := (&S3Prober{Keys: fakeKeys{}}).Probe(context.Background(), s3Profile(srv.URL))
	if out.Code != CodeTLS {
		t.Errorf("a certificate nobody trusts: outcome = %+v, want %s", out, CodeTLS)
	}

	profile := s3Profile(srv.URL)
	profile.Spec.S3.SkipTLSVerify = true
	if out := (&S3Prober{Keys: fakeKeys{}}).Probe(context.Background(), profile); !out.OK() {
		t.Errorf("with skipTLSVerify: outcome = %+v", out)
	}
}

func TestS3ProbeCredentialsProblems(t *testing.T) {
	out := (&S3Prober{Keys: fakeKeys{err: fmt.Errorf("%w: secret x does not exist", ErrCredentialsMissing)}}).Probe(context.Background(), s3Profile("http://127.0.0.1:1"))
	if out.Code != CodeCredentialsMissing {
		t.Errorf("outcome = %+v", out)
	}
	out = (&S3Prober{Keys: fakeKeys{err: errors.New("forbidden")}}).Probe(context.Background(), s3Profile("http://127.0.0.1:1"))
	if out.Code != CodeTestFailed {
		t.Errorf("an unexpected read error: outcome = %+v", out)
	}
}

func TestS3ProbeWithoutSettings(t *testing.T) {
	out := (&S3Prober{Keys: fakeKeys{}}).Probe(context.Background(), &LocationProfile{Spec: Spec{Type: TypeS3}})
	if out.OK() || out.Checks[0].Phase != snapshot.PhaseFailed {
		t.Errorf("outcome = %+v", out)
	}
}

func TestS3ProbeSpotsAnAddressWithTheWrongProtocol(t *testing.T) {
	plain := httptest.NewServer(&fakeS3{objects: map[string][]byte{}})
	defer plain.Close()
	secure := httptest.NewTLSServer(&fakeS3{objects: map[string][]byte{}})
	defer secure.Close()

	tests := []struct {
		name     string
		endpoint string
		skipTLS  bool
	}{
		{"https address of a server that speaks plain http", strings.Replace(plain.URL, "http://", "https://", 1), false},
		{"http address of a server that speaks https", strings.Replace(secure.URL, "https://", "http://", 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := s3Profile(tt.endpoint)
			profile.Spec.S3.SkipTLSVerify = tt.skipTLS
			out := (&S3Prober{Keys: fakeKeys{}, Timeout: 5 * time.Second}).Probe(context.Background(), profile)
			if out.Code != CodeProtocolMismatch {
				t.Errorf("outcome = %+v, want %s", out, CodeProtocolMismatch)
			}
		})
	}
}
