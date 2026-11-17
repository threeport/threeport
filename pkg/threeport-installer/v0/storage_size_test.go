package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestTheTierSizesTheControlPlanesVolumes covers the volumes a control
// plane's message broker servers and database nodes get: 5Gi and 20Gi on the
// development tier, as they always have, 250Gi and 1Ti on production, and the
// sizes given in place of either.
func TestTheTierSizesTheControlPlanesVolumes(t *testing.T) {
	for _, c := range []struct {
		name                    string
		opts                    Options
		messageBroker, database string
	}{
		{"development", Options{Tier: ControlPlaneTierDev}, "5Gi", "20Gi"},
		{"no tier", Options{}, "5Gi", "20Gi"},
		{"production", Options{Tier: ControlPlaneTierProd}, "250Gi", "1Ti"},
		{"production, sized", Options{Tier: ControlPlaneTierProd, MessageBrokerStorageSize: "500Gi", DatabaseStorageSize: "2Ti"}, "500Gi", "2Ti"},
		{"development, sized", Options{Tier: ControlPlaneTierDev, MessageBrokerStorageSize: "10Gi", DatabaseStorageSize: "50Gi"}, "10Gi", "50Gi"},
	} {
		cpi := &ControlPlaneInstaller{Opts: c.opts}
		assert.Equal(t, c.messageBroker, cpi.messageBrokerStorageSize(), c.name)
		assert.Equal(t, c.database, cpi.databaseStorageSize(), c.name)
	}
}

// TestAStorageSizeIsAPositiveQuantity covers the sizes tptctl up takes.
func TestAStorageSizeIsAPositiveQuantity(t *testing.T) {
	for _, size := range []string{"", "250Gi", "1Ti", "500G", "1.5Ti"} {
		assert.NoError(t, ValidateStorageSize("database-storage-size", size), size)
	}
	assert.ErrorContains(t, ValidateStorageSize("database-storage-size", "lots"), `--database-storage-size "lots" is not a size such as 250Gi`)
	assert.ErrorContains(t, ValidateStorageSize("message-broker-storage-size", "0"), "--message-broker-storage-size 0 is not a size greater than zero")
	assert.ErrorContains(t, ValidateStorageSize("message-broker-storage-size", "-5Gi"), "greater than zero")
}

// installedVolumeClaim installs the control plane dependencies into a fake
// cluster and returns the datadir claim template spec of the named
// StatefulSet, and the client, for the classes it created.
func installedVolumeClaims(t *testing.T, opts Options, objects ...runtime.Object) (map[string]map[string]interface{}, *dynamicfake.FakeDynamicClient) {
	t.Helper()

	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
	mapper := dependenciesMapper()
	opts.Name = ControlPlaneName
	opts.Namespace = ControlPlaneNamespace
	opts.RestApiInfo = ThreeportRestApi
	opts.DatabaseMigratorInfo = DatabaseMigrator
	opts.AgentInfo = ThreeportAgent
	opts.AdditionalOptions = map[string]interface{}{}
	opts.CreateOrUpdateKubeResources = true
	cpi := &ControlPlaneInstaller{Opts: opts}
	dbCreds := sharedDbCreds(t, ControlPlaneNamespace)
	seedBrokerSecrets(t, cpi, roundTripClient{kubeClient})
	require.NoError(t, cpi.InstallThreeportControlPlaneDependencies(roundTripClient{kubeClient}, &mapper, "", dbCreds))

	claims := map[string]map[string]interface{}{}
	for _, name := range []string{natsServiceName, "crdb"} {
		statefulSet, err := kubeClient.Resource(statefulSetGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err, name)
		templates, _, _ := unstructured.NestedSlice(statefulSet.Object, "spec", "volumeClaimTemplates")
		require.Len(t, templates, 1, name)
		spec, _, _ := unstructured.NestedMap(templates[0].(map[string]interface{}), "spec")
		claims[name] = spec
	}

	return claims, kubeClient
}

// claimSize returns the storage a claim template spec requests.
func claimSize(t *testing.T, spec map[string]interface{}) string {
	t.Helper()
	size, _, _ := unstructured.NestedString(spec, "resources", "requests", "storage")
	return size
}

// TestAnInstallsVolumesTakeTheirSizes covers the volume claims an install
// creates: the message broker's and the database's, each of its size, and on
// EKS from a class that lets them be grown in place, which the gp2 class EKS
// creates doesn't.
func TestAnInstallsVolumesTakeTheirSizes(t *testing.T) {
	claims, kubeClient := installedVolumeClaims(t, Options{Tier: ControlPlaneTierDev})
	assert.Equal(t, "5Gi", claimSize(t, claims[natsServiceName]))
	assert.Equal(t, "20Gi", claimSize(t, claims["crdb"]))
	assert.NotContains(t, claims["crdb"], "storageClassName", "elsewhere, the cluster's default class")
	_, err := kubeClient.Resource(storageClassGVR).Get(context.Background(), ControlPlaneStorageClassName, metav1.GetOptions{})
	assert.Error(t, err, "no class of threeport's off EKS")

	claims, _ = installedVolumeClaims(t, Options{Tier: ControlPlaneTierDev, MessageBrokerStorageSize: "8Gi", DatabaseStorageSize: "40Gi"})
	assert.Equal(t, "8Gi", claimSize(t, claims[natsServiceName]))
	assert.Equal(t, "40Gi", claimSize(t, claims["crdb"]))

	claims, kubeClient = installedVolumeClaims(t, Options{Tier: ControlPlaneTierDev, InfraProvider: v0.KubernetesRuntimeInfraProviderEKS})
	for name, claim := range claims {
		assert.Equal(t, ControlPlaneStorageClassName, claim["storageClassName"], name)
	}
	class, err := kubeClient.Resource(storageClassGVR).Get(context.Background(), ControlPlaneStorageClassName, metav1.GetOptions{})
	require.NoError(t, err)
	expandable, _, _ := unstructured.NestedBool(class.Object, "allowVolumeExpansion")
	assert.True(t, expandable)
	bindingMode, _, _ := unstructured.NestedString(class.Object, "volumeBindingMode")
	assert.Equal(t, "WaitForFirstConsumer", bindingMode, "a production database's volumes follow its pods across zones")
	assert.Empty(t, class.GetAnnotations()[defaultStorageClassAnnotation], "the cluster's default is left as it is")

	// a class of that name already there, as another control plane's on the
	// cluster, is left as it is
	existing := testStorageClass(ControlPlaneStorageClassName, false)
	existing.Object["parameters"] = map[string]interface{}{"type": "io2"}
	_, kubeClient = installedVolumeClaims(t, Options{Tier: ControlPlaneTierDev, InfraProvider: v0.KubernetesRuntimeInfraProviderEKS}, existing)
	class, err = kubeClient.Resource(storageClassGVR).Get(context.Background(), ControlPlaneStorageClassName, metav1.GetOptions{})
	require.NoError(t, err)
	volumeType, _, _ := unstructured.NestedString(class.Object, "parameters", "type")
	assert.Equal(t, "io2", volumeType)
}
