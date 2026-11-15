package v0

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestInstallRequestedModuleCreatesTheJob checks the stored request becomes a job.
func TestInstallRequestedModuleCreatesTheJob(t *testing.T) {
	// seed the encryption key the install config reads
	namespace := "primary-test"
	mapper := WorkloadRESTMapper()
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      EncryptionKeySecret,
			"namespace": namespace,
		},
		"data": map[string]interface{}{
			encryption.KeyEnvVar: base64.StdEncoding.EncodeToString([]byte("example-key")),
		},
	}}
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), secret)
	request := ModuleInstallRequest(&v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
	})
	name := "primary-test"
	endpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{
		Name:              &name,
		Namespace:         &namespace,
		ApiServerEndpoint: &endpoint,
		ModuleInstall:     &request,
	}

	// create the job from the stored request
	err := InstallRequestedModule(
		kubeClient,
		&mapper,
		instance,
		NewInstaller(Namespace(namespace)),
	)
	require.NoError(t, err)
	job, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).Get(
		context.Background(), "module-install-example", metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, "module-install-example", job.GetName())
}

// TestModuleInstallAfterRunKeepsAReplacement checks a different request stays stored.
func TestModuleInstallAfterRunKeepsAReplacement(t *testing.T) {
	// a stored request that is not the one this run started stays
	got := ModuleInstallAfterRun("example.com/other registry.example def456", "example.com/example-module-api registry.example abc123")
	require.Equal(t, "example.com/other registry.example def456", got)

	// the request this run started is cleared
	got = ModuleInstallAfterRun("example.com/example-module-api registry.example abc123", "example.com/example-module-api registry.example abc123")
	require.Equal(t, "", got)
}

// TestInstallRequestedModuleSkipsAnEmptyRequest checks a blank field creates nothing.
func TestInstallRequestedModuleSkipsAnEmptyRequest(t *testing.T) {
	// call with a row that stores no install request
	mapper := WorkloadRESTMapper()
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	namespace := "primary-test"
	instance := &v0.ControlPlaneInstance{Namespace: &namespace}

	err := InstallRequestedModule(
		kubeClient,
		&mapper,
		instance,
		NewInstaller(Namespace(namespace)),
	)
	require.NoError(t, err)
}
