package location

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/util/validation"
)

// MaxNameLength bounds the title of a location.
const MaxNameLength = 64

// ErrInvalid is returned for input that cannot be used. The message names the field.
var ErrInvalid = errors.New("invalid location")

func invalid(field, reason string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalid, field, reason)
}

// Input is what a person enters when they create or change a location. The keys are write-only: nothing the
// interface receives ever contains them.
type Input struct {
	Name string   `json:"name"`
	Type Type     `json:"type"`
	S3   *S3Input `json:"s3,omitempty"`
	NFS  *NFSSpec `json:"nfs,omitempty"`
}

// S3Input is an S3Spec together with the keys. Leaving both keys out when a location is changed keeps the stored ones.
type S3Input struct {
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region,omitempty"`
	Bucket        string `json:"bucket"`
	Prefix        string `json:"prefix,omitempty"`
	PathStyle     bool   `json:"pathStyle"`
	SkipTLSVerify bool   `json:"skipTLSVerify"`
	AccessKey     string `json:"accessKey,omitempty"`
	SecretKey     string `json:"secretKey,omitempty"`
}

// normalize trims the input and checks everything that does not need the cluster. It returns the spec to store,
// without keys and without the credentials secret, which the service fills in.
func (in *Input) normalize() (Spec, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > MaxNameLength {
		return Spec{}, invalid("name", fmt.Sprintf("use 1 to %d characters", MaxNameLength))
	}
	spec := Spec{Name: in.Name, Type: in.Type}

	switch in.Type {
	case TypeS3:
		if in.S3 == nil || in.NFS != nil {
			return Spec{}, invalid("type", "an S3 location needs the s3 settings and no nfs settings")
		}
		s3, err := in.S3.normalize()
		if err != nil {
			return Spec{}, err
		}
		spec.S3 = &s3
	case TypeNFS:
		if in.NFS == nil || in.S3 != nil {
			return Spec{}, invalid("type", "an NFS location needs the nfs settings and no s3 settings")
		}
		nfs, err := normalizeNFS(*in.NFS)
		if err != nil {
			return Spec{}, err
		}
		spec.NFS = &nfs
	default:
		return Spec{}, invalid("type", "must be s3 or nfs")
	}
	return spec, nil
}

func (in *S3Input) normalize() (S3Spec, error) {
	endpoint, err := NormalizeEndpoint(in.Endpoint)
	if err != nil {
		return S3Spec{}, err
	}
	bucket := strings.TrimSpace(in.Bucket)
	if !validBucket(bucket) {
		return S3Spec{}, invalid("bucket", "use 3 to 63 lowercase letters, digits, dots or hyphens")
	}
	prefix := strings.Trim(strings.TrimSpace(in.Prefix), "/")
	if len(prefix) > 512 || strings.Contains(prefix, "//") || strings.Contains(prefix, "..") || strings.ContainsAny(prefix, "\x00\\") {
		return S3Spec{}, invalid("prefix", "use plain folder names separated by /")
	}
	region := strings.TrimSpace(in.Region)
	if len(region) > 64 || strings.ContainsAny(region, " /\x00") {
		return S3Spec{}, invalid("region", "use a region name such as eu-central-1")
	}
	if (in.AccessKey == "") != (in.SecretKey == "") {
		return S3Spec{}, invalid("accessKey", "give the access key and the secret key together, or neither")
	}
	if len(in.AccessKey) > 512 || len(in.SecretKey) > 512 {
		return S3Spec{}, invalid("accessKey", "too long")
	}
	return S3Spec{
		Endpoint: endpoint, Region: region, Bucket: bucket, Prefix: prefix,
		PathStyle: in.PathStyle, SkipTLSVerify: in.SkipTLSVerify,
	}, nil
}

func validBucket(name string) bool {
	if len(name) < 3 || len(name) > 63 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '.' || r == '-') && i > 0 && i < len(name)-1:
		default:
			return false
		}
	}
	return true
}

// NormalizeEndpoint turns what a person typed into scheme://host[:port]. A missing scheme means https.
func NormalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", invalid("endpoint", "give the address of the storage")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", invalid("endpoint", "this is not an address")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", invalid("endpoint", "the address must start with http:// or https://")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", invalid("endpoint", "give only the host and port, without a path")
	}
	return u.Scheme + "://" + u.Host, nil
}

func normalizeNFS(in NFSSpec) (NFSSpec, error) {
	server := strings.TrimSpace(in.Server)
	if net.ParseIP(server) == nil && (server == "" || len(validation.IsDNS1123Subdomain(server)) > 0) {
		return NFSSpec{}, invalid("server", "use the host name or IP address of the NFS server")
	}
	path := strings.TrimSpace(in.Path)
	if !strings.HasPrefix(path, "/") || len(path) > 255 || strings.Contains(path, "..") || strings.ContainsAny(path, "\x00\\") {
		return NFSSpec{}, invalid("path", "give the exported directory as an absolute path, for example /exports/restora")
	}
	return NFSSpec{Server: server, Path: path}, nil
}
