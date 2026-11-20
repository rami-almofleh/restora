package location

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://s3.eu-central-1.amazonaws.com", "https://s3.eu-central-1.amazonaws.com", false},
		{"http://minio.minio:9000", "http://minio.minio:9000", false},
		{"  minio.example.com:9000  ", "https://minio.example.com:9000", false},
		{"https://minio.example.com/", "https://minio.example.com", false},
		{"http://10.0.0.5:9000", "http://10.0.0.5:9000", false},
		{"", "", true},
		{"ftp://example.com", "", true},
		{"https://example.com/bucket", "", true},
		{"https://user:pass@example.com", "", true},
		{"https://example.com?x=1", "", true},
		{"https://", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := NormalizeEndpoint(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("NormalizeEndpoint(%q) = %q, %v; want %q, error %v", tt.in, got, err, tt.want, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Errorf("error %v must wrap ErrInvalid", err)
			}
		})
	}
}

func TestInputNormalizeS3(t *testing.T) {
	in := s3Input("  Cloud  ")
	in.S3.Endpoint = "minio.example.com:9000"
	in.S3.Prefix = " /backups/prod/ "
	in.S3.Region = " eu-central-1 "

	spec, err := in.normalize()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "Cloud" || spec.S3.Endpoint != "https://minio.example.com:9000" || spec.S3.Prefix != "backups/prod" || spec.S3.Region != "eu-central-1" {
		t.Errorf("spec = %+v, s3 = %+v", spec, spec.S3)
	}
	if spec.S3.CredentialsSecret != "" {
		t.Error("the secret name is for the service to fill in, and the keys must never reach the spec")
	}
}

func TestInputNormalizeRejects(t *testing.T) {
	change := func(f func(*Input)) Input {
		in := s3Input("x")
		f(&in)
		return in
	}
	tests := []struct {
		name string
		in   Input
		want string
	}{
		{"empty name", change(func(in *Input) { in.Name = "  " }), "name"},
		{"long name", change(func(in *Input) { in.Name = strings.Repeat("a", MaxNameLength+1) }), "name"},
		{"unknown type", change(func(in *Input) { in.Type = "ftp" }), "type"},
		{"s3 without settings", Input{Name: "x", Type: TypeS3}, "type"},
		{"s3 with nfs settings too", change(func(in *Input) { in.NFS = &NFSSpec{Server: "a", Path: "/b"} }), "type"},
		{"nfs without settings", Input{Name: "x", Type: TypeNFS}, "type"},
		{"bad endpoint", change(func(in *Input) { in.S3.Endpoint = "ftp://x" }), "endpoint"},
		{"short bucket", change(func(in *Input) { in.S3.Bucket = "ab" }), "bucket"},
		{"capital bucket", change(func(in *Input) { in.S3.Bucket = "MyBucket" }), "bucket"},
		{"bucket with dot at the end", change(func(in *Input) { in.S3.Bucket = "bucket." }), "bucket"},
		{"prefix with ..", change(func(in *Input) { in.S3.Prefix = "a/../b" }), "prefix"},
		{"prefix with double slash", change(func(in *Input) { in.S3.Prefix = "a//b" }), "prefix"},
		{"region with space", change(func(in *Input) { in.S3.Region = "eu central" }), "region"},
		{"only the access key", change(func(in *Input) { in.S3.SecretKey = "" }), "accessKey"},
		{"only the secret key", change(func(in *Input) { in.S3.AccessKey = "" }), "accessKey"},
		{"nfs relative path", Input{Name: "x", Type: TypeNFS, NFS: &NFSSpec{Server: "nfs.example.com", Path: "exports"}}, "path"},
		{"nfs path with ..", Input{Name: "x", Type: TypeNFS, NFS: &NFSSpec{Server: "nfs.example.com", Path: "/a/../b"}}, "path"},
		{"nfs bad server", Input{Name: "x", Type: TypeNFS, NFS: &NFSSpec{Server: "not a host", Path: "/a"}}, "server"},
		{"nfs empty server", Input{Name: "x", Type: TypeNFS, NFS: &NFSSpec{Path: "/a"}}, "server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.in.normalize()
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want+":") {
				t.Errorf("error = %v, want ErrInvalid naming %q", err, tt.want)
			}
		})
	}
}

func TestInputNormalizeNFS(t *testing.T) {
	for _, server := range []string{"nfs.example.com", "10.1.2.3", "fd00::1", "nfs"} {
		in := nfsInput("x")
		in.NFS.Server = server
		if _, err := in.normalize(); err != nil {
			t.Errorf("server %q: %v", server, err)
		}
	}
}
