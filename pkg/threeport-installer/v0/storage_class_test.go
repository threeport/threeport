package v0

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// storageClassTestClient returns a fake cluster holding the given storage
// classes.
func storageClassTestClient(classes ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{storageClassGVR: "StorageClassList"},
		classes...,
	)
}

// testStorageClass returns a storage class, marked as the default or not.
func testStorageClass(name string, isDefault bool) *unstructured.Unstructured {
	class := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion":  "storage.k8s.io/v1",
		"kind":        "StorageClass",
		"metadata":    map[string]interface{}{"name": name},
		"provisioner": "kubernetes.io/aws-ebs",
	}}
	if isDefault {
		class.SetAnnotations(map[string]string{defaultStorageClassAnnotation: "true"})
	}

	return class
}

// TestAnEksClusterWithNoDefaultGetsOne covers #640: EKS no longer marks gp2 as
// the default, so a claim naming no class stayed pending. The class created is
// gp3 from the EBS CSI driver threeport installs, bound once its pod is
// scheduled so the volume is made in the pod's zone.
func TestAnEksClusterWithNoDefaultGetsOne(t *testing.T) {
	kubeClient := storageClassTestClient(testStorageClass("gp2", false))

	created, err := EnsureEksDefaultStorageClass(kubeClient)
	require.NoError(t, err)
	assert.True(t, created)

	class, err := kubeClient.Resource(storageClassGVR).Get(context.Background(), EksDefaultStorageClassName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "true", class.GetAnnotations()[defaultStorageClassAnnotation])
	assert.Equal(t, "ebs.csi.aws.com", class.Object["provisioner"])
	assert.Equal(t, map[string]interface{}{"type": "gp3"}, class.Object["parameters"])
	assert.Equal(t, "WaitForFirstConsumer", class.Object["volumeBindingMode"])
	assert.Equal(t, "Delete", class.Object["reclaimPolicy"])
	assert.Equal(t, true, class.Object["allowVolumeExpansion"])

	// the existing class is left as it was
	gp2, err := kubeClient.Resource(storageClassGVR).Get(context.Background(), "gp2", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, gp2.GetAnnotations())
}

// TestAClusterThatHasADefaultKeepsIt covers a default somebody else set,
// which threeport does not replace or add a second to.
func TestAClusterThatHasADefaultKeepsIt(t *testing.T) {
	kubeClient := storageClassTestClient(testStorageClass("their-default", true))

	created, err := EnsureEksDefaultStorageClass(kubeClient)
	require.NoError(t, err)
	assert.False(t, created)
	_, err = kubeClient.Resource(storageClassGVR).Get(context.Background(), EksDefaultStorageClassName, metav1.GetOptions{})
	assert.Error(t, err, "a second default would make which class a claim gets depend on the cluster")
}

// TestOurClassWithItsDefaultMarkerRemovedIsLeftAlone covers an operator who
// unmarked the class threeport created: it is not marked again on the next
// reconcile.
func TestOurClassWithItsDefaultMarkerRemovedIsLeftAlone(t *testing.T) {
	kubeClient := storageClassTestClient(testStorageClass(EksDefaultStorageClassName, false))

	created, err := EnsureEksDefaultStorageClass(kubeClient)
	require.NoError(t, err)
	assert.False(t, created)
	class, err := kubeClient.Resource(storageClassGVR).Get(context.Background(), EksDefaultStorageClassName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Empty(t, class.GetAnnotations())
}

// TestEnsuringTheDefaultIsIdempotent covers the runtime controller, which
// runs this on every reconcile.
func TestEnsuringTheDefaultIsIdempotent(t *testing.T) {
	kubeClient := storageClassTestClient()

	created, err := EnsureEksDefaultStorageClass(kubeClient)
	require.NoError(t, err)
	assert.True(t, created)
	created, err = EnsureEksDefaultStorageClass(kubeClient)
	require.NoError(t, err)
	assert.False(t, created)
}

func TestACreateThatFailsIsReported(t *testing.T) {
	kubeClient := storageClassTestClient()
	kubeClient.PrependReactor("create", "storageclasses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})

	_, err := EnsureEksDefaultStorageClass(kubeClient)
	assert.ErrorContains(t, err, "failed to create default storage class "+EksDefaultStorageClassName)
}

// TestADefaultMarkedTheOlderWayIsKept covers a class marked with the beta
// annotation, which Kubernetes still treats as the default: a second default
// beside it would take every claim that names no class.
func TestADefaultMarkedTheOlderWayIsKept(t *testing.T) {
	theirs := testStorageClass("their-default", false)
	theirs.SetAnnotations(map[string]string{betaDefaultStorageClassAnnotation: "true"})
	kubeClient := storageClassTestClient(theirs)

	created, err := EnsureEksDefaultStorageClass(kubeClient)
	require.NoError(t, err)
	assert.False(t, created)
	_, err = kubeClient.Resource(storageClassGVR).Get(context.Background(), EksDefaultStorageClassName, metav1.GetOptions{})
	assert.Error(t, err)
}

// TestOnlyTrueMarksADefault covers the value Kubernetes reads: a marker set
// to anything but "true", on either key, is not a default.
func TestOnlyTrueMarksADefault(t *testing.T) {
	for _, annotations := range []map[string]string{
		{defaultStorageClassAnnotation: "false"},
		{betaDefaultStorageClassAnnotation: "false"},
		{defaultStorageClassAnnotation: "True"},
	} {
		class := testStorageClass("not-default", false)
		class.SetAnnotations(annotations)
		assert.False(t, isDefaultStorageClass(*class), "%v", annotations)
	}
}

// bindingStorageClass returns a storage class with a volume binding mode,
// unset when mode is empty, marked as the default or not.
func bindingStorageClass(name, mode string, isDefault bool) *unstructured.Unstructured {
	class := testStorageClass(name, isDefault)
	if mode != "" {
		class.Object["volumeBindingMode"] = mode
	}

	return class
}

// TestAProductionDatabasesVolumesFollowItsPods covers the storage class a
// production database's volumes come from: one that creates a volume in the
// zone its pod is scheduled to, since a volume created first can hold a
// replica to a zone another replica has.
func TestAProductionDatabasesVolumesFollowItsPods(t *testing.T) {
	defaultClaim := map[string]interface{}{}
	gp2Claim := map[string]interface{}{"storageClassName": "gp2"}

	for _, c := range []struct {
		name    string
		claim   map[string]interface{}
		classes []runtime.Object
		refused string
	}{
		{"a default that waits for its pod", defaultClaim, []runtime.Object{bindingStorageClass("standard-rwo", "WaitForFirstConsumer", true)}, ""},
		{"a default that binds immediately", defaultClaim, []runtime.Object{bindingStorageClass("standard", "Immediate", true)}, "storage class standard creates volumes before their pods are scheduled (volumeBindingMode Immediate)"},
		{"a default with no binding mode", defaultClaim, []runtime.Object{bindingStorageClass("standard", "", true)}, "(volumeBindingMode Immediate)"},
		{"the class the claims name, over the default", gp2Claim, []runtime.Object{
			bindingStorageClass("gp2", "WaitForFirstConsumer", false), bindingStorageClass("standard", "Immediate", true),
		}, ""},
		{"a named class that binds immediately", gp2Claim, []runtime.Object{bindingStorageClass("gp2", "Immediate", false)}, "storage class gp2"},
		{"a named class that isn't there", gp2Claim, nil, ""},
		{"no default", defaultClaim, []runtime.Object{bindingStorageClass("standard", "Immediate", false)}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := checkDatabaseVolumeBinding(storageClassTestClient(c.classes...), c.claim)
			if c.refused == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, c.refused)
		})
	}
}

// TestOnlyAProductionInstallChecksTheDatabasesStorage covers where the check
// applies: a production install on a cluster whose default binds immediately
// stops before it creates the database, and a development one, with a single
// replica to place, goes ahead.
func TestOnlyAProductionInstallChecksTheDatabasesStorage(t *testing.T) {
	immediate := bindingStorageClass("standard", "Immediate", true)

	kubeClient, err := installDependenciesOn(t, backupInstaller(), immediate)
	assert.ErrorContains(t, err, "storage class standard creates volumes before their pods are scheduled")
	_, getErr := kubeClient.Resource(statefulSetGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), "crdb", metav1.GetOptions{})
	assert.True(t, k8serrors.IsNotFound(getErr), "the database was created")

	development := backupInstaller()
	development.Opts.Tier = ControlPlaneTier(ControlPlaneTierDev)
	development.Opts.GcpBackupBucket = ""
	_, err = installDependenciesOn(t, development, bindingStorageClass("standard", "Immediate", true))
	assert.NoError(t, err)
}
