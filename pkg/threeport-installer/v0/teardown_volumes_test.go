package v0

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

// TestDeletedVolumeHandlesAreTheDisksThatGoWithTheirClaims covers the disks a
// teardown cleans up when the cluster is deleted before the driver deletes
// them: exactly those of the namespace's claims that go with their claims.
func TestDeletedVolumeHandlesAreTheDisksThatGoWithTheirClaims(t *testing.T) {
	kubeClient := testKubeClient(
		testCsiPersistentVolume("pvc-crdb", teardownTestNamespace, "Delete", "projects/p/zones/us-east1-b/disks/pvc-crdb"),
		testCsiPersistentVolume("pvc-nats", teardownTestNamespace, "Delete", "projects/p/zones/us-east1-c/disks/pvc-nats"),
		testCsiPersistentVolume("pvc-kept", teardownTestNamespace, "Retain", "projects/p/zones/us-east1-b/disks/pvc-kept"),
		testCsiPersistentVolume("pvc-elsewhere", "someone-elses", "Delete", "projects/p/zones/us-east1-b/disks/pvc-elsewhere"),
		testPersistentVolume("pvc-no-disk", teardownTestNamespace, "Delete"),
	)

	handles, err := DeletedVolumeHandles(context.Background(), kubeClient, teardownTestNamespace)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"projects/p/zones/us-east1-b/disks/pvc-crdb",
		"projects/p/zones/us-east1-c/disks/pvc-nats",
	}, handles, "every volume that goes with the namespace's claims, by its disk's id, and none that is kept, belongs to another namespace or names no disk")
}

// fastKubeRequests shortens the deadline of each teardown request for the
// length of a test.
func fastKubeRequests(t *testing.T) {
	t.Helper()
	previous := kubeRequestTimeout
	kubeRequestTimeout = 100 * time.Millisecond
	t.Cleanup(func() { kubeRequestTimeout = previous })
}

// TestTheUninstallEndsWhenTheApiServerStopsAnswering covers an API server
// that accepts each of the uninstall's requests and never answers it, through
// a client with no timeout of its own, as the one a normal teardown makes:
// every request has to be bounded, or the teardown hangs.
func TestTheUninstallEndsWhenTheApiServerStopsAnswering(t *testing.T) {
	fastKubeRequests(t)
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
	mapper := testNamespaceMapper()

	for name, call := range map[string]func() error{
		"listing the volumes": func() error {
			_, err := volumesDeletedWithNamespace(kubeClient, teardownTestNamespace)
			return err
		},
		"deleting the namespace": func() error {
			return DeleteNamespaces(kubeClient, &mapper, []string{teardownTestNamespace})
		},
	} {
		done := make(chan error, 1)
		go func() { done <- call() }()
		select {
		case err := <-done:
			assert.Error(t, err, name)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not end with an API server that stopped answering", name)
		}
	}
}

// TestANamespaceThatCannotBeCheckedIsNotTakenAsDeleted covers the wait for a
// namespace: one the API server cannot be asked about is not gone, and the
// teardown does not go on as if it were.
func TestANamespaceThatCannotBeCheckedIsNotTakenAsDeleted(t *testing.T) {
	kubeClient := testKubeClient(testNamespace(teardownTestNamespace, ""))
	kubeClient.PrependReactor("delete", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	kubeClient.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection reset")
	})

	previousAttempts, previousPoll := namespaceDeleteAttempts, namespaceDeletePollSeconds
	namespaceDeleteAttempts, namespaceDeletePollSeconds = 3, 0
	t.Cleanup(func() { namespaceDeleteAttempts, namespaceDeletePollSeconds = previousAttempts, previousPoll })

	mapper := testNamespaceMapper()
	err := DeleteNamespaces(kubeClient, &mapper, []string{teardownTestNamespace})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check namespace")
}
