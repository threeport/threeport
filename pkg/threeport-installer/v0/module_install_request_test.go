package v0

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// installControlPlaneNamespace is the namespace the example control plane runs in.
const installControlPlaneNamespace = "primary-test"

// installJobName is the job the tests expect for the example module on that control plane.
var installJobName = moduleInstallJobName("example", installControlPlaneNamespace)

// installSecretName is the config secret that job mounts.
var installSecretName = moduleInstallSecretName("example", installControlPlaneNamespace)

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

	// create the job from the stored request on a control plane with no controllers
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil
	finished, err := InstallRequestedModule(
		kubeClient,
		&mapper,
		instance,
		cpi,
	)
	require.NoError(t, err)
	require.False(t, finished)
	job, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(installControlPlaneNamespace).Get(
		context.Background(), installJobName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, installJobName, job.GetName())

	// assert a request with no namespace leaves the flag off the job
	require.NotContains(t, installContainerArgs(t, job), "--"+ModuleNamespaceFlag)

	// assert the config secret sits in the control plane namespace, beside the job
	require.True(t, resourceExists(t, kubeClient, secretsResource, namespace, installSecretName))

	// assert the job runs as the control plane controller account, with no deployment
	account := ThreeportControlPlaneControllerName
	require.True(t, resourceExists(t, kubeClient, serviceAccountsResource, namespace, account))
	require.True(t, resourceExists(
		t, kubeClient, roleBindingsResource, "", namespace+"-"+account+"-threeportworkloads",
	))
	require.True(t, resourceExists(
		t, kubeClient, roleBindingsResource, "", namespace+"-"+account+"-"+clusterAdminClusterRoleName,
	))
	require.True(t, resourceExists(t, kubeClient, clusterRolesResource, "", threeportWorkloadsClusterRoleName))
	require.False(t, resourceExists(t, kubeClient, deploymentsResource, namespace, account))

	// assert the job brings no account of its own
	require.False(t, resourceExists(t, kubeClient, serviceAccountsResource, namespace, "module-install"))
	require.False(t, resourceExists(t, kubeClient, roleBindingsResource, "", "module-install-primary-test"))
}

// secretsResource, serviceAccountsResource, and roleBindingsResource name the
// kinds the install tests look for.
var (
	secretsResource         = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	serviceAccountsResource = schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}
	deploymentsResource     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	roleBindingsResource    = schema.GroupVersionResource{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings",
	}
	clusterRolesResource = schema.GroupVersionResource{
		Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles",
	}
)

// resourceExists reports whether an object is present. An empty namespace reads
// a cluster-scoped object.
func resourceExists(
	t *testing.T,
	client dynamic.Interface,
	resource schema.GroupVersionResource,
	namespace, name string,
) bool {
	t.Helper()

	// read the object and tell a missing one from a failed read
	var err error
	if namespace == "" {
		_, err = client.Resource(resource).Get(context.Background(), name, metav1.GetOptions{})
	} else {
		_, err = client.Resource(resource).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	}
	if k8serrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)

	return true
}

// TestControlPlaneControllerAccountBindsTheWorkloadIdentityUser covers a GKE
// control plane. The account annotation names the Google service account, and
// each binding lists the user the apiserver authenticates.
func TestControlPlaneControllerAccountBindsTheWorkloadIdentityUser(t *testing.T) {
	// create the account on a control plane that lists no controllers
	namespace := "primary-test"
	mapper := WorkloadRESTMapper()
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil
	cpi.Opts.InfraProvider = v0.KubernetesRuntimeInfraProviderGKE
	cpi.Opts.GcpProjectId = "demo-project"
	cpi.Opts.GcpServiceAccountEmail = "cp@demo-project.iam.gserviceaccount.com"
	require.NoError(t, cpi.ensureControlPlaneControllerAccount(kubeClient, &mapper, namespace))
	require.NoError(t, cpi.ensureControlPlaneControllerAccount(kubeClient, &mapper, namespace))

	// assert the account annotation names the Google service account
	account, err := kubeClient.Resource(serviceAccountsResource).Namespace(namespace).Get(
		context.Background(), ThreeportControlPlaneControllerName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, cpi.Opts.GcpServiceAccountEmail, account.GetAnnotations()["iam.gke.io/gcp-service-account"])

	// assert both bindings include the workload identity user
	user := fmt.Sprintf(
		"serviceAccount:%s.svc.id.goog[%s/%s]",
		cpi.Opts.GcpProjectId, namespace, ThreeportControlPlaneControllerName,
	)
	for _, suffix := range []string{"threeportworkloads", clusterAdminClusterRoleName} {
		binding, err := kubeClient.Resource(roleBindingsResource).Get(
			context.Background(),
			fmt.Sprintf("%s-%s-%s", namespace, ThreeportControlPlaneControllerName, suffix),
			metav1.GetOptions{},
		)
		require.NoError(t, err)
		subjects, _, err := unstructured.NestedSlice(binding.Object, "subjects")
		require.NoError(t, err)
		require.Len(t, subjects, 2)
		got, ok := subjects[1].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "User", got["kind"])
		require.Equal(t, user, got["name"])
	}

	// assert the call creates no controller deployment
	require.False(t, resourceExists(
		t, kubeClient, deploymentsResource, namespace, ThreeportControlPlaneControllerName,
	))
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
	jobs := kubeClient.Resource(schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}).Namespace(installControlPlaneNamespace)
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
		job, err := jobs.Get(context.Background(), installJobName, metav1.GetOptions{})
		require.NoError(t, err)
		if field == "failed" {
			require.NoError(t, unstructured.SetNestedSlice(job.Object, failedConditions(), "status", "conditions"))
		} else {
			require.NoError(t, unstructured.SetNestedField(job.Object, int64(1), "status", field))
		}
		_, err = jobs.Update(context.Background(), job, metav1.UpdateOptions{})
		require.NoError(t, err)
	}

	// assert the first pass creates the job and is not finished
	require.False(t, install())

	// assert a failed job is replaced by a new one, with the request still pending
	setStatus("failed")
	require.False(t, install())
	job, err := jobs.Get(context.Background(), installJobName, metav1.GetOptions{})
	require.NoError(t, err)
	_, conditioned, _ := unstructured.NestedSlice(job.Object, "status", "conditions")
	require.False(t, conditioned, "the failed job was kept instead of replaced")

	// assert a succeeded job finishes the request and is removed with its config secret
	require.True(t, resourceExists(t, kubeClient, secretsResource, installControlPlaneNamespace, installSecretName))
	setStatus("succeeded")
	require.True(t, install())
	_, err = jobs.Get(context.Background(), installJobName, metav1.GetOptions{})
	require.Error(t, err, "the succeeded job was kept")
	require.False(t, resourceExists(t, kubeClient, secretsResource, installControlPlaneNamespace, installSecretName), "the config secret was kept")
}

// TestInstallRequestedModuleGivesUpAfterRepeatedFailures checks the limit on a
// failing install: each failure makes the next attempt, the third failure
// returns ErrModuleInstallFailed and marks the job abandoned, and a later
// request creates a new job at the first attempt.
func TestInstallRequestedModuleGivesUpAfterRepeatedFailures(t *testing.T) {

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
	jobs := kubeClient.Resource(schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}).Namespace(installControlPlaneNamespace)
	name := "primary-test"
	endpoint := "https://api.example"
	request := ModuleInstallRequest(&v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
	})
	install := func() error {
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
	failJob := func() {
		job, err := jobs.Get(context.Background(), installJobName, metav1.GetOptions{})
		require.NoError(t, err)
		require.NoError(t, unstructured.SetNestedSlice(job.Object, failedConditions(), "status", "conditions"))
		_, err = jobs.Update(context.Background(), job, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	attempt := func() string {
		job, err := jobs.Get(context.Background(), installJobName, metav1.GetOptions{})
		require.NoError(t, err)
		return job.GetAnnotations()[moduleInstallAttemptsAnnotation]
	}

	// assert each failure is replaced by the next attempt
	require.NoError(t, install())
	require.Equal(t, "1", attempt())
	failJob()
	require.NoError(t, install())
	require.Equal(t, "2", attempt())
	failJob()
	require.NoError(t, install())
	require.Equal(t, "3", attempt())

	// assert the third failure gives up and keeps the job for its logs
	failJob()
	require.ErrorIs(t, install(), ErrModuleInstallFailed)
	job, err := jobs.Get(context.Background(), installJobName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "true", job.GetAnnotations()[moduleInstallAbandonedAnnotation])

	// assert the give-up removes the config secret, which holds the child's credentials
	require.False(t, resourceExists(t, kubeClient, secretsResource, installControlPlaneNamespace, installSecretName), "the config secret was kept")

	// assert a later request creates a new job at the first attempt
	require.NoError(t, install())
	require.Equal(t, "1", attempt())
}

// failedConditions returns the conditions Kubernetes sets on a job that has used
// its retries.
func failedConditions() []interface{} {
	return []interface{}{map[string]interface{}{"type": "Failed", "status": "True"}}
}

// TestAJobWithAPendingRetryIsNotFailed checks that a job whose pod failed but
// whose retry has not started is left running. Only the Failed condition marks a
// job as failed, so the request keeps its attempt and the retry runs.
func TestAJobWithAPendingRetryIsNotFailed(t *testing.T) {
	// a failed pod count with no active pod and no Failed condition
	retrying := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{"failed": int64(1)},
	}}
	require.False(t, jobFailed(retrying))

	// the Failed condition marks the job failed
	exhausted := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{"failed": int64(2), "conditions": failedConditions()},
	}}
	require.True(t, jobFailed(exhausted))

	// a succeeded job is never failed
	succeeded := &unstructured.Unstructured{Object: map[string]interface{}{
		"status": map[string]interface{}{"succeeded": int64(1), "conditions": failedConditions()},
	}}
	require.False(t, jobFailed(succeeded))
}

// installContainerArgs reads the install container arguments from a job.
func installContainerArgs(t *testing.T, job *unstructured.Unstructured) []string {
	t.Helper()

	// read the install container
	containers, _, err := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.NotEmpty(t, containers)
	container := containers[0].(map[string]interface{})
	raw, _, err := unstructured.NestedSlice(container, "args")
	require.NoError(t, err)

	// copy each argument out as a string
	args := make([]string, len(raw))
	for i, arg := range raw {
		args[i] = fmt.Sprint(arg)
	}
	return args
}

// installKeySecret returns the encryption key the install config reads.
func installKeySecret(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
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
}

// installExampleRequest returns a stored install for the example module.
func installExampleRequest(moduleNamespace string) string {
	return ModuleInstallRequest(&v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
	}, moduleNamespace)
}

// TestModuleInstallRequestStoresANamespace checks the stored request shape.
func TestModuleInstallRequestStoresANamespace(t *testing.T) {
	// a request with no namespace stays three fields
	stored := installExampleRequest("")
	require.Equal(t, "example.com/example-module-api registry.example abc123", stored)
	_, moduleNamespace, err := moduleFromInstallRequest(stored)
	require.NoError(t, err)
	require.Empty(t, moduleNamespace)

	// a namespace is stored after the tag and read back
	stored = installExampleRequest("threeport-example-b")
	require.Equal(t, "example.com/example-module-api registry.example abc123 threeport-example-b", stored)
	_, moduleNamespace, err = moduleFromInstallRequest(stored)
	require.NoError(t, err)
	require.Equal(t, "threeport-example-b", moduleNamespace)

	// an endpoint namespace is stored when the caller passes none
	endpoint := "example-api.custom-ns.svc.cluster.local"
	recorded := &v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
		Endpoint:       &endpoint,
	}
	stored = ModuleInstallRequest(recorded)
	require.Equal(t, "example.com/example-module-api registry.example abc123 custom-ns", stored)

	// a passed namespace wins over the endpoint
	stored = ModuleInstallRequest(recorded, "threeport-example-b")
	require.Equal(t, "example.com/example-module-api registry.example abc123 threeport-example-b", stored)

	// the default namespace is left off so an older plugin still runs
	endpoint = "example-api.threeport-example.svc.cluster.local"
	recorded.Endpoint = &endpoint
	stored = ModuleInstallRequest(recorded)
	require.Equal(t, "example.com/example-module-api registry.example abc123", stored)
	stored = ModuleInstallRequest(recorded, "threeport-example")
	require.Equal(t, "example.com/example-module-api registry.example abc123", stored)
}

// TestEnsureModuleInstallJobPassesTheGivenNamespace checks the job passes the
// namespace it was given.
func TestEnsureModuleInstallJobPassesTheGivenNamespace(t *testing.T) {
	// seed the key, and a module whose endpoint names a custom namespace
	namespace := installControlPlaneNamespace
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(installKeySecret(namespace))
	endpoint := "example-api.custom-ns.svc.cluster.local"
	module := &v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
		Endpoint:       &endpoint,
	}
	name := namespace
	apiEndpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{
		Name:              &name,
		Namespace:         &namespace,
		ApiServerEndpoint: &apiEndpoint,
	}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil

	// create the job in the namespace the caller already decided
	finished, err := EnsureModuleInstallJob(
		kubeClient, &mapper, instance, cpi, module, "custom-ns",
	)
	require.NoError(t, err)
	require.False(t, finished)
	job, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).Get(context.Background(), installJobName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{
		"install",
		"--control-plane-image-namespace",
		"registry.example",
		"--control-plane-image-tag",
		"abc123",
		"--" + ModuleNamespaceFlag,
		"custom-ns",
	}, installContainerArgs(t, job))
}

// TestRetagModuleAPIInstallsWhereTheEndpointRecords checks a missing API
// deployment is installed in the namespace the endpoint records.
func TestRetagModuleAPIInstallsWhereTheEndpointRecords(t *testing.T) {
	// seed the key, and a module whose endpoint names a custom namespace
	namespace := installControlPlaneNamespace
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(installKeySecret(namespace))
	endpoint := "example-api.custom-ns.svc.cluster.local"
	module := &v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
		Endpoint:       &endpoint,
	}
	name := namespace
	apiEndpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{
		Name:              &name,
		Namespace:         &namespace,
		ApiServerEndpoint: &apiEndpoint,
	}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil

	// install the missing deployment where the endpoint records
	err := retagModuleAPI(
		kubeClient, &mapper, instance, cpi, module, "registry.example", "abc123", namespace,
	)
	require.NoError(t, err)
	job, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).Get(context.Background(), installJobName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{
		"install",
		"--control-plane-image-namespace",
		"registry.example",
		"--control-plane-image-tag",
		"abc123",
		"--" + ModuleNamespaceFlag,
		"custom-ns",
	}, installContainerArgs(t, job))
}

// TestEnsureModuleInstallJobOmitsTheDefaultNamespace checks a repair in the
// default namespace does not pass the namespace flag.
func TestEnsureModuleInstallJobOmitsTheDefaultNamespace(t *testing.T) {
	// seed the key, and a module whose endpoint names the default namespace
	namespace := installControlPlaneNamespace
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(installKeySecret(namespace))
	endpoint := "example-api.threeport-example.svc.cluster.local"
	module := &v0.ModuleApi{
		Name:           util.Ptr("example.com/example-module-api"),
		ImageNamespace: util.Ptr("registry.example"),
		ImageTag:       util.Ptr("abc123"),
		Endpoint:       &endpoint,
	}
	name := namespace
	apiEndpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{
		Name:              &name,
		Namespace:         &namespace,
		ApiServerEndpoint: &apiEndpoint,
	}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil

	// create the job without the namespace flag
	finished, err := EnsureModuleInstallJob(kubeClient, &mapper, instance, cpi, module, "")
	require.NoError(t, err)
	require.False(t, finished)
	job, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).Get(context.Background(), installJobName, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotContains(t, installContainerArgs(t, job), "--"+ModuleNamespaceFlag)
}

// TestInstallRequestedModuleRejectsATakenNamespace checks that a taken namespace stops a new install.
func TestInstallRequestedModuleRejectsATakenNamespace(t *testing.T) {
	// seed the default namespace with no owner label
	namespace := installControlPlaneNamespace
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(testModuleNamespace("threeport-example", "uid"))
	request := installExampleRequest("")
	name := namespace
	endpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{
		Name:              &name,
		Namespace:         &namespace,
		ApiServerEndpoint: &endpoint,
		ModuleInstall:     &request,
	}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil

	// refuse the install and leave the job uncreated
	finished, err := InstallRequestedModule(kubeClient, &mapper, instance, cpi)
	require.ErrorIs(t, err, ErrModuleNamespaceExists)
	require.False(t, finished)
	require.False(t, resourceExists(t, kubeClient, schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}, namespace, installJobName))
}

// TestInstallRequestedModuleAcceptsAnotherNamespace checks that a free namespace still installs.
func TestInstallRequestedModuleAcceptsAnotherNamespace(t *testing.T) {
	// seed the default namespace, and leave the named namespace absent
	namespace := installControlPlaneNamespace
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(
		testModuleNamespace("threeport-example", "uid"),
		installKeySecret(namespace),
	)
	request := installExampleRequest("threeport-example-b")
	name := namespace
	endpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{
		Name:              &name,
		Namespace:         &namespace,
		ApiServerEndpoint: &endpoint,
		ModuleInstall:     &request,
	}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil

	// create the job and pass the named namespace
	finished, err := InstallRequestedModule(kubeClient, &mapper, instance, cpi)
	require.NoError(t, err)
	require.False(t, finished)
	job, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}).Namespace(namespace).Get(context.Background(), installJobName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{
		"install",
		"--control-plane-image-namespace",
		"registry.example",
		"--control-plane-image-tag",
		"abc123",
		"--" + ModuleNamespaceFlag,
		"threeport-example-b",
	}, installContainerArgs(t, job))
}

// TestInstallRequestedModuleKeepsItsOwnNamespace checks that a retry continues
// when this control plane already labeled the namespace.
func TestInstallRequestedModuleKeepsItsOwnNamespace(t *testing.T) {
	// seed the default namespace labeled for this control plane
	namespace := installControlPlaneNamespace
	owned := testModuleNamespace("threeport-example", "uid")
	owned.SetLabels(map[string]string{ModuleNamespaceOwnerLabel: namespace})
	mapper := WorkloadRESTMapper()
	kubeClient := testKubeClient(owned, installKeySecret(namespace))
	request := installExampleRequest("")
	name := namespace
	endpoint := "https://api.example"
	instance := &v0.ControlPlaneInstance{
		Name:              &name,
		Namespace:         &namespace,
		ApiServerEndpoint: &endpoint,
		ModuleInstall:     &request,
	}
	cpi := NewInstaller(Namespace(namespace))
	cpi.Opts.ControllerList = nil

	// create the job without the namespace flag
	finished, err := InstallRequestedModule(kubeClient, &mapper, instance, cpi)
	require.NoError(t, err)
	require.False(t, finished)
	require.True(t, resourceExists(t, kubeClient, schema.GroupVersionResource{
		Group: "batch", Version: "v1", Resource: "jobs",
	}, namespace, installJobName))
}
