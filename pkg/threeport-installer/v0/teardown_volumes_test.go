package v0

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

// teardownTestNamespace is the control plane namespace the teardown tests use.
const teardownTestNamespace = "threeport-control-plane"

// testPersistentVolume returns a volume bound to a claim in namespace.
func testPersistentVolume(name, namespace, reclaimPolicy string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolume",
		"metadata":   map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"persistentVolumeReclaimPolicy": reclaimPolicy,
			"claimRef": map[string]interface{}{
				"namespace": namespace,
				"name":      "datadir-" + name,
			},
		},
	}}
}

// fastVolumeWait shortens the wait for volumes for the length of a test.
func fastVolumeWait(t *testing.T, timeout time.Duration) {
	t.Helper()
	previousTimeout, previousInterval := volumeDeleteTimeout, volumeDeletePollInterval
	volumeDeleteTimeout, volumeDeletePollInterval = timeout, 10*time.Millisecond
	t.Cleanup(func() {
		volumeDeleteTimeout, volumeDeletePollInterval = previousTimeout, previousInterval
	})
}

func TestTheVolumesWaitedForAreTheNamespacesDeletedOnes(t *testing.T) {
	kubeClient := testKubeClient(
		testPersistentVolume("pvc-crdb", teardownTestNamespace, "Delete"),
		testPersistentVolume("pvc-nats", teardownTestNamespace, "Delete"),
		testPersistentVolume("pvc-kept", teardownTestNamespace, "Retain"),
		testPersistentVolume("pvc-elsewhere", "someone-elses", "Delete"),
	)

	volumes, err := volumesDeletedWithNamespace(kubeClient, teardownTestNamespace)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pvc-crdb", "pvc-nats"}, volumes,
		"a retained volume outlives its claim on purpose, and another namespace's is not the control plane's")
}

// TestUninstallWaitsForTheDisksBehindItsClaims covers #639: deleting the
// cluster before the storage driver deleted the disk behind a claim left the
// disk behind, unattached and billing. The uninstall returns only once the
// volumes are gone, which the driver allows only once it deleted the disk.
func TestUninstallWaitsForTheDisksBehindItsClaims(t *testing.T) {
	fastVolumeWait(t, 5*time.Second)
	kubeClient := testKubeClient(
		testNamespace(teardownTestNamespace, ""),
		testPersistentVolume("pvc-crdb", teardownTestNamespace, "Delete"),
	)

	// the driver deletes the volume a while after its claim is gone
	var gets atomic.Int32
	kubeClient.PrependReactor("get", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
		if gets.Add(1) == 5 {
			require.NoError(t, kubeClient.Tracker().Delete(persistentVolumeGVR, "", "pvc-crdb"))
		}

		return false, nil, nil
	})

	mapper := testNamespaceMapper()
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: teardownTestNamespace}}
	require.NoError(t, cpi.UnInstallThreeportControlPlaneComponents(kubeClient, &mapper))

	assert.GreaterOrEqual(t, gets.Load(), int32(5), "the uninstall returned before the volume was deleted")
	_, err := kubeClient.Resource(persistentVolumeGVR).Get(context.Background(), "pvc-crdb", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestUninstallReportsAVolumeThatIsNotDeleted(t *testing.T) {
	fastVolumeWait(t, 100*time.Millisecond)
	kubeClient := testKubeClient(
		testNamespace(teardownTestNamespace, ""),
		testPersistentVolume("pvc-stuck", teardownTestNamespace, "Delete"),
	)

	mapper := testNamespaceMapper()
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: teardownTestNamespace}}
	err := cpi.UnInstallThreeportControlPlaneComponents(kubeClient, &mapper)
	require.Error(t, err, "deleting the cluster now would leave the disk behind")
	assert.Contains(t, err.Error(), "pvc-stuck")
}

func TestUninstallReportsANamespaceThatIsNotDeleted(t *testing.T) {
	kubeClient := testKubeClient(testNamespace(teardownTestNamespace, ""))
	// the namespace is never removed, as one held by a finalizer
	kubeClient.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, testNamespace(teardownTestNamespace, ""), nil
	})
	kubeClient.PrependReactor("delete", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})

	previousAttempts, previousPoll := namespaceDeleteAttempts, namespaceDeletePollSeconds
	namespaceDeleteAttempts, namespaceDeletePollSeconds = 3, 0
	t.Cleanup(func() { namespaceDeleteAttempts, namespaceDeletePollSeconds = previousAttempts, previousPoll })

	mapper := testNamespaceMapper()
	err := DeleteNamespaces(kubeClient, &mapper, []string{teardownTestNamespace})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace "+teardownTestNamespace+" was not deleted")
}

func TestUninstallReportsAVolumeItCannotCheck(t *testing.T) {
	fastVolumeWait(t, time.Second)
	kubeClient := testKubeClient(testPersistentVolume("pvc-crdb", teardownTestNamespace, "Delete"))
	kubeClient.PrependReactor("get", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})

	err := waitForVolumesDeleted(kubeClient, []string{"pvc-crdb"})
	assert.ErrorContains(t, err, "failed to check persistent volume pvc-crdb")
}

// TestTheWaitEndsWhenTheApiServerStopsAnswering covers an API server that
// accepts a request and never answers it: the deadline has to bound the
// request, not only the polling between requests, or the teardown hangs.
func TestTheWaitEndsWhenTheApiServerStopsAnswering(t *testing.T) {
	fastVolumeWait(t, 200*time.Millisecond)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	kubeClient, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- waitForVolumesDeleted(kubeClient, []string{"pvc-crdb"}) }()
	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pvc-crdb")
	case <-time.After(5 * time.Second):
		t.Fatal("the wait for volumes did not end with an API server that stopped answering")
	}
}

// TestTheListingEndsWhenTheApiServerStopsAnswering covers the listing of the
// volumes to wait for, which runs before the wait: an API server that accepts
// the request and never answers it would otherwise hang the teardown there.
func TestTheListingEndsWhenTheApiServerStopsAnswering(t *testing.T) {
	fastVolumeWait(t, 200*time.Millisecond)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); server.Close() })
	kubeClient, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := volumesDeletedWithNamespace(kubeClient, teardownTestNamespace)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("listing the volumes did not end with an API server that stopped answering")
	}
}

// TestAWaitThatRunsOutNamesOnlyTheVolumesStillThere covers the error a
// deadline ends the wait with. It named every volume the wait started with,
// so a volume already confirmed deleted - by an earlier pass, or earlier in
// the pass the deadline cut short - was reported as left behind, sending an
// operator after a disk that was gone.
func TestAWaitThatRunsOutNamesOnlyTheVolumesStillThere(t *testing.T) {
	for name, test := range map[string]struct {
		volumes []string
		// the attempt from which a volume's check answers it is gone, or
		// hangs until the wait runs out
		goneFrom, hangsFrom map[string]int
		named, notNamed     []string
	}{
		"gone earlier in the pass the deadline cut short": {
			volumes:   []string{"pvc-deleted", "pvc-stuck", "pvc-unchecked"},
			goneFrom:  map[string]int{"pvc-deleted": 2},
			hangsFrom: map[string]int{"pvc-stuck": 2},
			named:     []string{"pvc-stuck", "pvc-unchecked"},
			notNamed:  []string{"pvc-deleted"},
		},
		"gone in an earlier pass, and after the volume the deadline cut short": {
			volumes:   []string{"pvc-stuck", "pvc-deleted"},
			goneFrom:  map[string]int{"pvc-deleted": 2},
			hangsFrom: map[string]int{"pvc-stuck": 3},
			named:     []string{"pvc-stuck"},
			notNamed:  []string{"pvc-deleted"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fastVolumeWait(t, 300*time.Millisecond)
			var mu sync.Mutex
			gets := map[string]int{}
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				volume := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				mu.Lock()
				gets[volume]++
				attempt := gets[volume]
				mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				switch {
				case test.goneFrom[volume] != 0 && attempt >= test.goneFrom[volume]:
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
				case test.hangsFrom[volume] != 0 && attempt >= test.hangsFrom[volume]:
					select {
					case <-release:
					case <-r.Context().Done():
					}
				default:
					_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"PersistentVolume","metadata":{"name":"` + volume + `"}}`))
				}
			}))
			t.Cleanup(func() { close(release); server.Close() })
			kubeClient, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
			require.NoError(t, err)

			err = waitForVolumesDeleted(kubeClient, test.volumes)
			require.Error(t, err)
			for _, volume := range test.named {
				assert.Contains(t, err.Error(), volume, "still there when last checked")
			}
			for _, volume := range test.notNamed {
				assert.NotContains(t, err.Error(), volume, "confirmed gone before the wait ran out")
			}
		})
	}
}

// testCsiPersistentVolume returns a volume provisioned by a CSI driver for a
// claim in namespace.
func testCsiPersistentVolume(name, namespace, reclaimPolicy, handle string) *unstructured.Unstructured {
	volume := testPersistentVolume(name, namespace, reclaimPolicy)
	volume.Object["spec"].(map[string]interface{})["csi"] = map[string]interface{}{
		"driver":       "ebs.csi.aws.com",
		"volumeHandle": handle,
	}

	return volume
}

// TestRetainedVolumeHandlesAreTheDisksKeptAfterTheirClaims covers the disks a
// teardown's cleanup has to leave alone.
func TestRetainedVolumeHandlesAreTheDisksKeptAfterTheirClaims(t *testing.T) {
	inTree := testPersistentVolume("pvc-in-tree", teardownTestNamespace, "Retain")
	inTree.Object["spec"].(map[string]interface{})["awsElasticBlockStore"] = map[string]interface{}{
		"volumeID": "aws://us-east-1a/vol-in-tree",
	}
	kubeClient := testKubeClient(
		testCsiPersistentVolume("pvc-crdb", teardownTestNamespace, "Delete", "vol-crdb"),
		testCsiPersistentVolume("pvc-kept", teardownTestNamespace, "Retain", "vol-kept"),
		testCsiPersistentVolume("pvc-recycled", teardownTestNamespace, "Recycle", "vol-recycled"),
		testCsiPersistentVolume("pvc-elsewhere", "someone-elses", "Retain", "vol-elsewhere"),
		inTree,
	)

	handles, err := RetainedVolumeHandles(context.Background(), kubeClient, teardownTestNamespace)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"vol-kept", "vol-recycled", "vol-in-tree"}, handles,
		"every volume the namespace's claims keep, by its disk's id, and none that goes with its claim or belongs to another namespace")
}
