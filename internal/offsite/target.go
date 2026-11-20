// Package offsite copies backups to storage locations outside the cluster and brings them back. Volumes and saved
// resources are stored with Kopia, which encrypts, deduplicates and only uploads what changed. Kopia runs in short-lived
// pods (the data movers) next to the data; the controller only orchestrates them.
package offsite

import (
	"net/url"
	"strings"

	"restora.local/restora/internal/location"
)

// repositoryFolder is the folder inside the bucket or share that holds the Kopia repository.
const repositoryFolder = "restora-repository"

// Environment variables that describe a repository to a data mover pod. The pod reads them from a secret.
const (
	envType      = "RESTORA_REPO_TYPE"
	envEndpoint  = "RESTORA_REPO_ENDPOINT"
	envInsecure  = "RESTORA_REPO_INSECURE"
	envSkipTLS   = "RESTORA_REPO_SKIP_TLS_VERIFY"
	envBucket    = "RESTORA_REPO_BUCKET"
	envPrefix    = "RESTORA_REPO_PREFIX"
	envRegion    = "RESTORA_REPO_REGION"
	envAccessKey = "RESTORA_REPO_ACCESS_KEY"
	envSecretKey = "RESTORA_REPO_SECRET_KEY"
	envPassword  = "RESTORA_REPO_PASSWORD"
	envNFSMount  = "RESTORA_REPO_NFS_MOUNT"
)

// NFSMountPath is where the pod of a data mover mounts the share of an NFS location.
const NFSMountPath = "/repo-nfs"

// Target is everything needed to open the repository of one storage location.
type Target struct {
	// Location is the name of the LocationProfile.
	Location string
	Type     location.Type
	S3       *location.S3Spec
	NFS      *location.NFSSpec
	// AccessKey and SecretKey are only set for S3.
	AccessKey, SecretKey string
	// Password is the key that encrypts the repository.
	Password string
}

// Env describes the target as environment variables for a data mover pod.
func (t Target) Env() map[string]string {
	env := map[string]string{envType: string(t.Type), envPassword: t.Password}
	if t.S3 != nil {
		host, insecure := splitEndpoint(t.S3.Endpoint)
		env[envEndpoint] = host
		env[envBucket] = t.S3.Bucket
		env[envPrefix] = t.S3.Prefix
		env[envRegion] = t.S3.Region
		env[envAccessKey] = t.AccessKey
		env[envSecretKey] = t.SecretKey
		if insecure {
			env[envInsecure] = "true"
		}
		if t.S3.SkipTLSVerify {
			env[envSkipTLS] = "true"
		}
	}
	if t.NFS != nil {
		env[envNFSMount] = NFSMountPath
	}
	return env
}

// TargetFromEnv is the reverse of Env, used inside the data mover pod.
func TargetFromEnv(get func(string) string) Target {
	t := Target{Type: location.Type(get(envType)), Password: get(envPassword)}
	switch t.Type {
	case location.TypeS3:
		scheme := "https"
		if get(envInsecure) == "true" {
			scheme = "http"
		}
		t.S3 = &location.S3Spec{
			Endpoint: scheme + "://" + get(envEndpoint), Bucket: get(envBucket), Prefix: get(envPrefix),
			Region: get(envRegion), SkipTLSVerify: get(envSkipTLS) == "true",
		}
		t.AccessKey, t.SecretKey = get(envAccessKey), get(envSecretKey)
	case location.TypeNFS:
		t.NFS = &location.NFSSpec{Path: get(envNFSMount)} // the mount point in the pod, not the path on the server
	}
	return t
}

// splitEndpoint turns scheme://host[:port] into host[:port] and whether the connection is unencrypted.
func splitEndpoint(endpoint string) (host string, insecure bool) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint, false
	}
	return u.Host, u.Scheme == "http"
}

// repoArgs returns the Kopia provider and its arguments for the target. They contain the keys, so they must only be
// passed to the process, never logged.
func (t Target) repoArgs() (provider string, args []string) {
	switch t.Type {
	case location.TypeNFS:
		// Inside a data mover pod the share is always mounted at NFSMountPath, whatever it is called on the server.
		return "filesystem", []string{"--path", NFSMountPath + "/" + repositoryFolder}
	default:
		host, insecure := splitEndpoint(t.S3.Endpoint)
		prefix := repositoryFolder + "/"
		if p := strings.Trim(t.S3.Prefix, "/"); p != "" {
			prefix = p + "/" + prefix
		}
		args = []string{"--bucket", t.S3.Bucket, "--prefix", prefix, "--endpoint", host, "--access-key", t.AccessKey, "--secret-access-key", t.SecretKey}
		if t.S3.Region != "" {
			args = append(args, "--region", t.S3.Region)
		}
		if insecure {
			args = append(args, "--disable-tls")
		}
		if t.S3.SkipTLSVerify {
			args = append(args, "--disable-tls-verification")
		}
		return "s3", args
	}
}
