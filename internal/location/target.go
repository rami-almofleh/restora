package location

import (
	"path"
	"strings"
)

// TargetKey identifies the place a location writes to: two locations with the same key would share one repository.
// Their repository keys differ, so the second one could never read what the first one wrote. It also would not be a
// second copy of anything.
func (s Spec) TargetKey() string {
	switch {
	case s.S3 != nil:
		endpoint := strings.ToLower(strings.TrimRight(strings.TrimSpace(s.S3.Endpoint), "/"))
		return "s3://" + endpoint + "/" + s.S3.Bucket + "/" + strings.Trim(path.Clean("/"+s.S3.Prefix), "/")
	case s.NFS != nil:
		return "nfs://" + strings.ToLower(strings.TrimSpace(s.NFS.Server)) + path.Clean("/"+s.NFS.Path)
	}
	return ""
}
