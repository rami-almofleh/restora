package location

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3Timeout bounds one test of an S3 location.
const s3Timeout = 30 * time.Second

// KeyReader returns the keys stored for a location. SecretReader implements it.
type KeyReader interface {
	Credentials(ctx context.Context, secretName string) (accessKey, secretKey string, err error)
}

// S3Prober tests S3-compatible storage: it lists the bucket, then writes, reads and deletes a small test object.
type S3Prober struct {
	Keys KeyReader
	// Timeout bounds one test. It defaults to 30 seconds.
	Timeout time.Duration
}

// Probe runs the test. It never returns an error: what went wrong is part of the Outcome.
func (p *S3Prober) Probe(ctx context.Context, l *LocationProfile) *Outcome {
	out := newOutcome(CheckConnect, CheckWrite, CheckRead, CheckDelete)
	spec := l.Spec.S3
	if spec == nil {
		return out.fail(CheckConnect, CodeTestFailed, "the location has no S3 settings", "Edit the location and fill in the S3 settings.")
	}

	accessKey, secretKey, err := p.Keys.Credentials(ctx, spec.CredentialsSecret)
	if err != nil {
		code, hint := CodeTestFailed, "Check that the Restora controller may read secrets and try again."
		if errors.Is(err, ErrCredentialsMissing) {
			code, hint = CodeCredentialsMissing, "Edit the location and enter the access key and the secret key again."
		}
		return out.fail(CheckConnect, code, err.Error(), hint)
	}
	client, err := newS3Client(spec, accessKey, secretKey)
	if err != nil {
		return out.fail(CheckConnect, CodeEndpointUnreachable, err.Error(), "Check the address of the storage.")
	}

	timeout := p.Timeout
	if timeout == 0 {
		timeout = s3Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// A listing is a GET request, so the store tells wrong keys from missing rights in its answer. A HEAD request
	// to the bucket would answer "forbidden" for both.
	listCtx, stopList := context.WithCancel(ctx)
	defer stopList()
	for object := range client.ListObjects(listCtx, spec.Bucket, minio.ListObjectsOptions{Prefix: spec.Prefix, MaxKeys: 1}) {
		if object.Err != nil {
			return failFromS3(out, CheckConnect, object.Err)
		}
		break
	}
	out.pass(CheckConnect)

	key := path.Join(spec.Prefix, ".restora-test", randomName())
	payload := []byte("restora connection test " + key)
	if _, err := client.PutObject(ctx, spec.Bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{}); err != nil {
		return failFromS3(out, CheckWrite, err)
	}
	out.pass(CheckWrite)

	// Whatever happens next, do not leave the test object behind.
	defer func() {
		_ = client.RemoveObject(context.WithoutCancel(ctx), spec.Bucket, key, minio.RemoveObjectOptions{})
	}()

	object, err := client.GetObject(ctx, spec.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return failFromS3(out, CheckRead, err)
	}
	got, err := io.ReadAll(object)
	_ = object.Close()
	if err != nil {
		return failFromS3(out, CheckRead, err)
	}
	if !bytes.Equal(got, payload) {
		return out.fail(CheckRead, CodeTestFailed, "the object that was read back differs from the one that was written",
			"The storage returned other data than was stored. Check the storage for faults.")
	}
	out.pass(CheckRead)

	if err := client.RemoveObject(ctx, spec.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return failFromS3(out, CheckDelete, err)
	}
	out.pass(CheckDelete)
	return out
}

func randomName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand does not fail on supported platforms
	return hex.EncodeToString(b)
}

func newS3Client(spec *S3Spec, accessKey, secretKey string) (*minio.Client, error) {
	u, err := url.Parse(spec.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: spec.SkipTLSVerify} //nolint:gosec // an explicit, documented option for self-signed test setups
	lookup := minio.BucketLookupAuto
	if spec.PathStyle {
		lookup = minio.BucketLookupPath
	}
	return minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       u.Scheme == "https",
		Region:       spec.Region,
		BucketLookup: lookup,
		Transport:    transport,
	})
}

// failFromS3 records a failed step with the code that fits the error.
func failFromS3(out *Outcome, step string, err error) *Outcome {
	code, hint := classifyS3Error(err)
	return out.fail(step, code, err.Error(), hint)
}

// classifyS3Error turns an error of the storage or of the network into a stable code and an English hint.
func classifyS3Error(err error) (code, hint string) {
	switch minio.ToErrorResponse(err).Code {
	case "InvalidAccessKeyId", "SignatureDoesNotMatch", "InvalidToken", "ExpiredToken":
		return CodeInvalidCredentials, "Check the access key and the secret key."
	case "AccessDenied", "AllAccessDisabled", "AccountProblem":
		return CodeAccessDenied, "The keys are valid but may not list, write, read and delete in this bucket."
	case "NoSuchBucket", "InvalidBucketName":
		return CodeBucketNotFound, "Check the name of the bucket and create it if it does not exist."
	case "RequestTimeTooSkewed":
		return CodeTestFailed, "The clock of this cluster and the clock of the storage differ too much."
	}

	// http:// in front of a server that speaks https, or the other way round, is the commonest slip with a custom endpoint.
	if text := err.Error(); strings.Contains(text, "server gave HTTP response to HTTPS client") ||
		strings.Contains(text, "HTTP request to an HTTPS server") || strings.Contains(text, "first record does not look like a TLS handshake") {
		return CodeProtocolMismatch, "Switch the address between http:// and https://."
	}

	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalidCert x509.CertificateInvalidError
	var verification *tls.CertificateVerificationError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalidCert) || errors.As(err, &verification) {
		return CodeTLS, "The certificate of the storage is not trusted."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeTimeout, "The storage did not answer in time."
	}
	var netErr net.Error
	var urlErr *url.Error
	var dnsErr *net.DNSError
	if errors.As(err, &netErr) || errors.As(err, &urlErr) || errors.As(err, &dnsErr) {
		return CodeEndpointUnreachable, "Check the address, the port and whether the cluster can reach the storage."
	}
	return CodeTestFailed, "See the technical details."
}
