package v0

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
