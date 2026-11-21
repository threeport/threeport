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
