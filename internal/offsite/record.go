package offsite

import (
	"errors"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"restora.local/restora/internal/crstore"
	"restora.local/restora/internal/snapshot"
)

// Kind of the custom resource that records a backup stored in a storage location. It outlives the Snapshot it came
// from, so an application can be restored from the location after its snapshots, or the whole namespace, are gone.
const Kind = "Export"

// LabelLocation is set on every Export, next to the labels for application and snapshot.
const LabelLocation = "restora.local/location"

// GVR addresses the Export custom resource.
var GVR = schema.GroupVersionResource{Group: snapshot.Group, Version: snapshot.Version, Resource: "exports"}

// ErrNotFound is returned when an Export does not exist.
var ErrNotFound = errors.New("export not found")

// Record is the custom resource.
type Record struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RecordSpec `json:"spec"`
}

// RecordSpec says what is stored and where.
type RecordSpec struct {
	// Application is the namespace the backup was taken from.
	Application string `json:"application"`
	// Snapshot is the name of the local Snapshot the copy was made from. It may be gone.
	Snapshot string `json:"snapshot"`
	// Location is the name of the LocationProfile. LocationName is its title when the copy was made.
	Location     string `json:"location"`
	LocationName string `json:"locationName,omitempty"`
	// CreatedAt is when the backup was taken.
	CreatedAt metav1.Time         `json:"createdAt"`
	Policy    *snapshot.PolicyRef `json:"policy,omitempty"`
	// ResourcesSnapshot is the id of the saved resources in the repository.
	ResourcesSnapshot string                 `json:"resourcesSnapshot"`
	Resources         snapshot.ResourceStats `json:"resources"`
	Volumes           []RecordVolume         `json:"volumes"`
	// AddedBytes is how much the repository grew by this copy.
	AddedBytes int64 `json:"addedBytes,omitempty"`
}

// RecordVolume is one volume of the backup in the repository.
type RecordVolume struct {
	Claim         string `json:"claim"`
	KopiaSnapshot string `json:"kopiaSnapshot"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// SizeBytes is the size of all volumes of the backup.
func (s RecordSpec) SizeBytes() int64 {
	var total int64
	for _, v := range s.Volumes {
		total += v.SizeBytes
	}
	return total
}

// RecordName is the name of the Export for a snapshot and a location. It starts with the time of the snapshot, so
// lists in name order are in time order.
func RecordName(snapshotName, location string) string {
	stamp := strings.TrimPrefix(snapshotName, "s-")
	short := location[strings.LastIndex(location, "-")+1:]
	return "x-" + stamp + "-" + short
}

// RecordStore reads and writes Export resources in one namespace.
type RecordStore struct {
	*crstore.Store[Record, *Record]
}

// NewRecordStore creates a RecordStore for the namespace Restora runs in.
func NewRecordStore(client dynamic.Interface, namespace string) *RecordStore {
	return &RecordStore{crstore.New[Record](client, GVR, namespace, Kind, ErrNotFound)}
}
