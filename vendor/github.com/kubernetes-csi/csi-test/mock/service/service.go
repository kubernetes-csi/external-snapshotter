package service

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/csi-test/mock/cache"
	"golang.org/x/net/context"
)

const (
	// Name is the name of the CSI plug-in.
	Name = "io.kubernetes.storage.mock"

	// VendorVersion is the version returned by GetPluginInfo.
	VendorVersion = "0.3.0"
)

// Manifest is the SP's manifest.
var Manifest = map[string]string{
	"url": "https://github.com/kubernetes-csi/csi-test/mock",
}

// Service is the CSI Mock service provider.
type Service interface {
	csi.ControllerServer
	csi.IdentityServer
	csi.NodeServer
}

type service struct {
	sync.Mutex
	nodeID       string
	vols         []csi.Volume
	volsRWL      sync.RWMutex
	volsNID      uint64
	snapshots    cache.SnapshotCache
	snapshotsNID uint64
}

type Volume struct {
	sync.Mutex
	VolumeCSI       csi.Volume
	NodeID          string
	ISStaged        bool
	ISPublished     bool
	StageTargetPath string
	TargetPath      string
}

var MockVolumes map[string]Volume

// New returns a new Service.
func New() Service {
	s := &service{nodeID: Name}
	s.snapshots = cache.NewSnapshotCache()
	s.vols = []csi.Volume{
		s.newVolume("Mock Volume 1", gib100),
		s.newVolume("Mock Volume 2", gib100),
		s.newVolume("Mock Volume 3", gib100),
	}
	MockVolumes = map[string]Volume{}

	s.snapshots.Add(s.newSnapshot("Mock Snapshot 1", "1", map[string]string{"Description": "snapshot 1"}))
	s.snapshots.Add(s.newSnapshot("Mock Snapshot 2", "2", map[string]string{"Description": "snapshot 2"}))
	s.snapshots.Add(s.newSnapshot("Mock Snapshot 3", "3", map[string]string{"Description": "snapshot 3"}))

	return s
}

const (
	kib    int64 = 1024
	mib    int64 = kib * 1024
	gib    int64 = mib * 1024
	gib100 int64 = gib * 100
	tib    int64 = gib * 1024
	tib100 int64 = tib * 100
)

func (s *service) newVolume(name string, capcity int64) csi.Volume {
	return csi.Volume{
		Id:            fmt.Sprintf("%d", atomic.AddUint64(&s.volsNID, 1)),
		Attributes:    map[string]string{"name": name},
		CapacityBytes: capcity,
	}
}

func (s *service) findVol(k, v string) (volIdx int, volInfo csi.Volume) {
	s.volsRWL.RLock()
	defer s.volsRWL.RUnlock()
	return s.findVolNoLock(k, v)
}

func (s *service) findVolNoLock(k, v string) (volIdx int, volInfo csi.Volume) {
	volIdx = -1

	for i, vi := range s.vols {
		switch k {
		case "id":
			if strings.EqualFold(v, vi.Id) {
				return i, vi
			}
		case "name":
			if n, ok := vi.Attributes["name"]; ok && strings.EqualFold(v, n) {
				return i, vi
			}
		}
	}

	return
}

func (s *service) findVolByName(
	ctx context.Context, name string) (int, csi.Volume) {

	return s.findVol("name", name)
}

func (s *service) newSnapshot(name, sourceVolumeId string, parameters map[string]string) cache.Snapshot {
	return cache.Snapshot{
		Name:       name,
		Parameters: parameters,
		SnapshotCSI: csi.Snapshot{
			Id:             fmt.Sprintf("%d", atomic.AddUint64(&s.snapshotsNID, 1)),
			CreatedAt:      time.Now().UnixNano(),
			SourceVolumeId: sourceVolumeId,
			Status: &csi.SnapshotStatus{
				Type:    csi.SnapshotStatus_READY,
				Details: "snapshot ready",
			},
		},
	}
}
