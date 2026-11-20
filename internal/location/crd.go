// Package location manages the places backups are exported to (S3-compatible storage and NFS). The API creates
// LocationProfile resources and stores credentials as Kubernetes secrets; the controller tests the connection.
package location

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"restora.local/restora/internal/snapshot"
)

// Kind identifies the LocationProfile custom resource. Group and version are shared with the other Restora resources.
const Kind = "LocationProfile"

// GVR addresses the LocationProfile custom resource.
var GVR = schema.GroupVersionResource{Group: snapshot.Group, Version: snapshot.Version, Resource: "locationprofiles"}

// Secret layout. The API writes the secret, the controller reads it. The interface never gets the values back.
const (
	SecretPrefix    = "restora-location-"
	SecretKeyAccess = "accessKey"
	SecretKeySecret = "secretKey"
)

// Type is the kind of storage.
type Type string

const (
	TypeS3  Type = "s3"
	TypeNFS Type = "nfs"
)

// LocationProfile is the custom resource.
type LocationProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              Spec   `json:"spec"`
	Status            Status `json:"status,omitempty"`
}

// Spec says where backups go. Exactly one of S3 and NFS is set, matching Type.
type Spec struct {
	// Name is the title shown to people. The resource name is a generated id.
	Name string   `json:"name"`
	Type Type     `json:"type"`
	S3   *S3Spec  `json:"s3,omitempty"`
	NFS  *NFSSpec `json:"nfs,omitempty"`
	// TestRequestedAt asks the controller to test the connection. The API sets it when the profile is created or
	// changed and when somebody presses "test connection".
	TestRequestedAt *metav1.Time `json:"testRequestedAt,omitempty"`
}

// S3Spec describes an S3-compatible bucket. The keys are not part of it: they live in the secret named by
// CredentialsSecret, with the keys accessKey and secretKey.
type S3Spec struct {
	// Endpoint is scheme://host[:port], for example https://s3.eu-central-1.amazonaws.com or http://minio.minio:9000.
	Endpoint string `json:"endpoint"`
	Region   string `json:"region,omitempty"`
	Bucket   string `json:"bucket"`
	// Prefix is a folder inside the bucket. Everything of Restora lives below it.
	Prefix string `json:"prefix,omitempty"`
	// PathStyle addresses the bucket as host/bucket instead of bucket.host. MinIO and most self-hosted stores need it.
	PathStyle bool `json:"pathStyle,omitempty"`
	// SkipTLSVerify accepts any certificate. It is for self-signed test setups only.
	SkipTLSVerify     bool   `json:"skipTLSVerify,omitempty"`
	CredentialsSecret string `json:"credentialsSecret"`
}

// NFSSpec describes an NFS share.
type NFSSpec struct {
	Server string `json:"server"`
	// Path is the exported directory on the server, for example /exports/restora.
	Path string `json:"path"`
}

// Status reports what the controller found out.
type Status struct {
	LastTest *TestResult `json:"lastTest,omitempty"`
}

// TestResult is the outcome of one connection test.
type TestResult struct {
	// RequestedAt is the Spec.TestRequestedAt this result answers. It is unset for the first automatic test.
	RequestedAt *metav1.Time `json:"requestedAt,omitempty"`
	At          metav1.Time  `json:"at"`
	OK          bool         `json:"ok"`
	DurationMs  int64        `json:"durationMs"`
	// Checks are the steps of the test. Steps after a failure are Pending, which means "not tried".
	Checks []snapshot.StepStatus `json:"checks"`
	// ErrorCode is a stable identifier of the failure, for example bucket_not_found.
	ErrorCode string `json:"errorCode,omitempty"`
	Message   string `json:"message,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

// Test steps, shown in this order.
const (
	CheckConnect = "connect"
	CheckMount   = "mount"
	CheckWrite   = "write"
	CheckRead    = "read"
	CheckDelete  = "delete"
)

// Codes of a failed test, stored in TestResult.ErrorCode. The interface turns them into plain sentences.
const (
	CodeCredentialsMissing  = "credentials_missing"
	CodeInvalidCredentials  = "invalid_credentials"
	CodeAccessDenied        = "access_denied"
	CodeBucketNotFound      = "bucket_not_found"
	CodeEndpointUnreachable = "endpoint_unreachable"
	CodeTLS                 = "tls_error"
	CodeProtocolMismatch    = "protocol_mismatch"
	CodeTimeout             = "timeout"
	CodeNFSMountFailed      = "nfs_mount_failed"
	CodeNFSNotWritable      = "nfs_not_writable"
	CodeTestFailed          = "test_failed"
)
