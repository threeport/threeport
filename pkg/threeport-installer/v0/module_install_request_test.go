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

	// create the job from the stored request, which has not finished
	finished, err := InstallRequestedModule(
		kubeClient,
		&mapper,
		instance,
		NewInstaller(Namespace(namespace)),
	)
	require.NoError(t, err)
	require.False(t, finished)
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

	finished, err := InstallRequestedModule(
		kubeClient,
		&mapper,
		instance,
		NewInstaller(Namespace(namespace)),
	)
	require.NoError(t, err)
	require.True(t, finished)
}

// TestInstallRequestedModuleKeepsARequestForAnotherImagePending checks that a
// running install job satisfies only a request for its own image. A request for
// another image returns an error, so the controller keeps it stored until the
// running job ends.
func TestInstallRequestedModuleKeepsARequestForAnotherImagePending(t *testing.T) {
	// seed the key the install config reads
	namespace := "primary-test"
	mapper := WorkloadRESTMapper()
	keySecret := &unstructured.Unstructured{Object: map[string]interface{}{
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
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), keySecret)
	name := "primary-test"
	endpoint := "https://api.example"
	install := func(tag string) error {
		request := ModuleInstallRequest(&v0.ModuleApi{
			Name:           util.Ptr("example.com/example-module-api"),
			ImageNamespace: util.Ptr("registry.example"),
			ImageTag:       util.Ptr(tag),
		})
		_, err := InstallRequestedModule(
			kubeClient,
			&mapper,
			&v0.ControlPlaneInstance{
				Name:              &name,
				Namespace:         &namespace,
				ApiServerEndpoint: &endpoint,
				ModuleInstall:     &request,
			},
			NewInstaller(Namespace(namespace)),
		)
		return err
	}

	// start a job for the first image, which has not finished
	require.NoError(t, install("abc123"))

	// assert a request for the same image waits on the running job without error
	require.NoError(t, install("abc123"))

	// assert a request for another image stays pending
	require.ErrorIs(t, install("def456"), ErrModuleInstallRunning)
}

// TestInstallRequestedModuleFollowsTheJobToItsOutcome checks what a request does
// at each state of its install job: a succeeded job finishes the request and is
// removed, a failed job is replaced by a new one and the request stays pending.
func TestInstallRequestedModuleFollowsTheJobToItsOutcome(t *testing.T) {
	// seed the key the install config reads
	namespace := "primary-test"
	mapper := WorkloadRESTMapper()
	keySecret := &unstructured.Unstructured{Object: map[string]interface{}{
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
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), keySecret)
	jobs := kubeClient.Resource(schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}).Namespace(namespace)
	name := "primary-test"
	endpoint := "https://api.example"
	request := ModuleInstallRequest(&v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
	})
	install := func() bool {
		finished, err := InstallRequestedModule(
			kubeClient,
			&mapper,
			&v0.ControlPlaneInstance{
				Name:              &name,
				Namespace:         &namespace,
				ApiServerEndpoint: &endpoint,
				ModuleInstall:     &request,
			},
			NewInstaller(Namespace(namespace)),
		)
		require.NoError(t, err)
		return finished
	}
	setStatus := func(field string) {
		job, err := jobs.Get(context.Background(), "module-install-example", metav1.GetOptions{})
		require.NoError(t, err)
		require.NoError(t, unstructured.SetNestedField(job.Object, int64(1), "status", field))
		_, err = jobs.Update(context.Background(), job, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	// assert the first pass creates the job and is not finished
	require.False(t, install())

	// assert a failed job is replaced by a new one, with the request still pending
	setStatus("failed")
	require.False(t, install())
	job, err := jobs.Get(context.Background(), "module-install-example", metav1.GetOptions{})
	require.NoError(t, err)
	failed, _, _ := unstructured.NestedInt64(job.Object, "status", "failed")
	require.Zero(t, failed, "the failed job was kept instead of replaced")

	// assert a succeeded job finishes the request and is removed
	setStatus("succeeded")
	require.True(t, install())
	_, err = jobs.Get(context.Background(), "module-install-example", metav1.GetOptions{})
	require.Error(t, err, "the succeeded job was kept")
}
