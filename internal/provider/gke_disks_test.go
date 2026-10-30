package provider

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gce "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

// gkeDiskTestNamespace is the control plane namespace the disk tests use.
const gkeDiskTestNamespace = "threeport-control-plane"

// fakeGceDiskApi is a project's disks, by volume handle.
type fakeGceDiskApi struct {
	disks     map[string]*gce.Disk
	deleted   []string
	deleteErr error
}

func (f *fakeGceDiskApi) getDisk(_ context.Context, disk gceDisk) (*gce.Disk, error) {
	found, ok := f.disks[disk.String()]
	if !ok {
		return nil, &googleapi.Error{Code: http.StatusNotFound}
	}

	return found, nil
}

func (f *fakeGceDiskApi) deleteDisk(_ context.Context, disk gceDisk) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.disks, disk.String())
	f.deleted = append(f.deleted, disk.String())

	return nil
}

// provisionedDisk is a disk the GCE persistent disk driver provisioned for a
// claim in namespace.
func provisionedDisk(namespace string, users ...string) *gce.Disk {
	return &gce.Disk{
		Description: `{"kubernetes.io/created-for/pv/name":"pvc-1","kubernetes.io/created-for/pvc/name":"datadir-crdb-0","kubernetes.io/created-for/pvc/namespace":"` +
			namespace + `","storage.gke.io/created-by":"pd.csi.storage.gke.io"}`,
		Users: users,
	}
}

func TestParseGceDiskHandle(t *testing.T) {
	cases := []struct {
		handle string
		want   gceDisk
	}{
		{"projects/p/zones/us-east1-b/disks/pvc-1", gceDisk{project: "p", zone: "us-east1-b", name: "pvc-1"}},
		{"projects/p/regions/us-east1/disks/pvc-1", gceDisk{project: "p", region: "us-east1", name: "pvc-1"}},
		{"projects/UNSPECIFIED/zones/us-east1-b/disks/pvc-1", gceDisk{project: "cluster-project", zone: "us-east1-b", name: "pvc-1"}},
	}
	for _, c := range cases {
		disk, err := parseGceDiskHandle(c.handle, "cluster-project")
		require.NoError(t, err, c.handle)
		assert.Equal(t, c.want, disk, c.handle)
	}

	for _, handle := range []string{
		"vol-0123456789abcdef0",
		"projects/p/zones/us-east1-b/snapshots/pvc-1",
		"projects/p/locations/us-east1/disks/pvc-1",
		"projects/p/zones//disks/pvc-1",
		"projects/p/zones/us-east1-b/disks/",
		"projects/p/zones/us-east1-b/disks/pvc-1/extra",
	} {
		_, err := parseGceDiskHandle(handle, "cluster-project")
		assert.Error(t, err, "%q names no disk", handle)
	}
}

// TestOnlyTheNamedLeftoverDisksAreDeleted covers which disks the cleanup
// deletes: the ones the cluster named, still there, provisioned for the
// namespace's claims and unused - and no other disk in the project, even one
// provisioned for a claim in the same namespace by another cluster.
func TestOnlyTheNamedLeftoverDisksAreDeleted(t *testing.T) {
	diskApi := &fakeGceDiskApi{disks: map[string]*gce.Disk{
		"projects/p/zones/us-east1-b/disks/pvc-crdb":          provisionedDisk(gkeDiskTestNamespace),
		"projects/p/regions/us-east1/disks/pvc-nats":          provisionedDisk(gkeDiskTestNamespace),
		"projects/p/zones/us-east1-b/disks/pvc-in-use":        provisionedDisk(gkeDiskTestNamespace, "instances/node-1"),
		"projects/p/zones/us-east1-b/disks/pvc-other-ns":      provisionedDisk("someone-elses"),
		"projects/p/zones/us-east1-b/disks/hand-made":         {Description: "not provisioned by a driver"},
		"projects/p/zones/us-east1-b/disks/pvc-other-cluster": provisionedDisk(gkeDiskTestNamespace),
	}}

	deleted, err := deleteLeftoverGkeDisks(context.Background(), diskApi, "p", gkeDiskTestNamespace, []string{
		"projects/p/zones/us-east1-b/disks/pvc-crdb",
		"projects/p/regions/us-east1/disks/pvc-nats",
		"projects/p/zones/us-east1-b/disks/pvc-in-use",
		"projects/p/zones/us-east1-b/disks/pvc-other-ns",
		"projects/p/zones/us-east1-b/disks/hand-made",
		"projects/p/zones/us-east1-b/disks/pvc-already-gone",
	})

	assert.ElementsMatch(t, []string{
		"projects/p/zones/us-east1-b/disks/pvc-crdb",
		"projects/p/regions/us-east1/disks/pvc-nats",
	}, deleted)
	assert.ElementsMatch(t, deleted, diskApi.deleted)
	assert.Contains(t, diskApi.disks, "projects/p/zones/us-east1-b/disks/pvc-other-cluster",
		"a disk the cluster did not name is never deleted")
	require.Error(t, err, "the disks left alone are reported")
	assert.Contains(t, err.Error(), "pvc-in-use alone: it is in use by instances/node-1")
	assert.Contains(t, err.Error(), "pvc-other-ns alone: it was not provisioned for a claim in namespace "+gkeDiskTestNamespace)
	assert.Contains(t, err.Error(), "hand-made alone: it was not provisioned")
	assert.NotContains(t, err.Error(), "pvc-already-gone", "a disk already gone is not a failure")
}

func TestAFailedDiskDeletionIsReportedAndTheRestGoOn(t *testing.T) {
	diskApi := &fakeGceDiskApi{
		disks: map[string]*gce.Disk{
			"projects/p/zones/us-east1-b/disks/pvc-crdb": provisionedDisk(gkeDiskTestNamespace),
		},
		deleteErr: errors.New("quota"),
	}

	deleted, err := deleteLeftoverGkeDisks(context.Background(), diskApi, "p", gkeDiskTestNamespace, []string{
		"projects/p/zones/us-east1-b/disks/pvc-crdb",
		"vol-not-a-gce-disk",
	})

	assert.Empty(t, deleted)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete disk projects/p/zones/us-east1-b/disks/pvc-crdb: quota")
	assert.Contains(t, err.Error(), `"vol-not-a-gce-disk" is not a GCE persistent disk`)
}

func TestNoHandlesNeedsNoComputeClient(t *testing.T) {
	deleted, err := DeleteLeftoverGkeDisks(context.Background(), "p", gkeDiskTestNamespace, nil, "not json")
	assert.NoError(t, err, "with nothing to delete, no client is created, so credentials are never read")
	assert.Empty(t, deleted)
}
