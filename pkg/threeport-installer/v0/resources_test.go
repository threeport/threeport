package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// testComponentMapper returns a mapper for every kind the API server and agent
// installs create.
func testComponentMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "policy", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
		{Group: "apiextensions.k8s.io", Version: "v1"},
	})
	for _, gvk := range []schema.GroupVersionKind{
		{Version: "v1", Kind: "Secret"},
		{Version: "v1", Kind: "ConfigMap"},
		{Version: "v1", Kind: "Service"},
		{Version: "v1", Kind: "ServiceAccount"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	} {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	for _, gvk := range []schema.GroupVersionKind{
		{Version: "v1", Kind: "Namespace"},
		{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
	} {
		mapper.Add(gvk, meta.RESTScopeRoot)
	}

	return mapper
}

// testInstaller returns an installer for the default control plane.
func testInstaller() *ControlPlaneInstaller {
	return &ControlPlaneInstaller{Opts: Options{
		Name:                        ControlPlaneName,
		Namespace:                   ControlPlaneNamespace,
		ControllerList:              ThreeportControllerList,
		RestApiInfo:                 ThreeportRestApi,
		DatabaseMigratorInfo:        DatabaseMigrator,
		AgentInfo:                   ThreeportAgent,
		AdditionalOptions:           map[string]interface{}{},
		CreateOrUpdateKubeResources: true,
	}}
}

// getDeployment reads a Deployment back from a client.
func getDeployment(t *testing.T, kubeClient dynamic.Interface, namespace, name string) *unstructured.Unstructured {
	t.Helper()

	deployment, err := kubeClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)

	return deployment
}

// controlPlaneDeployments returns the API server, agent and controller
// Deployments an install creates, keyed by Deployment name.
func controlPlaneDeployments(t *testing.T, cpi *ControlPlaneInstaller) map[string]*unstructured.Unstructured {
	t.Helper()

	deployments := map[string]*unstructured.Unstructured{}

	dbCreds, err := auth.GenerateDbCreds(cpi.Opts.Namespace)
	require.NoError(t, err)
	apiClient := testKubeClient()
	apiMapper := testComponentMapper()
	require.NoError(t, cpi.UpdateThreeportAPIDeployment(apiClient, &apiMapper, dbCreds))
	deployments[ThreeportAPIServiceResourceName] = getDeployment(t, apiClient, cpi.Opts.Namespace, ThreeportAPIServiceResourceName)

	agentClient := testKubeClient()
	agentMapper := testComponentMapper()
	require.NoError(t, cpi.UpdateThreeportAgentDeployment(agentClient, &agentMapper))
	deployments[ThreeportAgentDeployName] = getDeployment(t, agentClient, cpi.Opts.Namespace, ThreeportAgentDeployName)

	for _, controller := range cpi.Opts.ControllerList {
		name := "threeport-" + controller.Name
		deployment, err := cpi.getControllerDeployment(name, cpi.Opts.Namespace, *controller)
		require.NoError(t, err)
		deployments[name] = deployment
	}

	return deployments
}

// podContainers returns a Deployment's containers and init containers, keyed
// by container name.
func podContainers(t *testing.T, deployment *unstructured.Unstructured) map[string]map[string]interface{} {
	t.Helper()

	containers := map[string]map[string]interface{}{}
	for _, field := range []string{"containers", "initContainers"} {
		list, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", field)
		require.NoError(t, err)
		for _, item := range list {
			container := item.(map[string]interface{})
			containers[container["name"].(string)] = container
		}
	}

	return containers
}

// TestEveryControlPlaneContainerHasResources covers the guarantee the
// resources exist for: a container without requests is scheduled as if it
// needs nothing, and one without limits can take its whole node down with it.
// It reads every container of every Deployment, so a container added later
// without resources fails here rather than shipping.
func TestEveryControlPlaneContainerHasResources(t *testing.T) {
	deployments := controlPlaneDeployments(t, testInstaller())
	require.Len(t, deployments, len(ThreeportControllerList)+2)

	for deploymentName, deployment := range deployments {
		for containerName, container := range podContainers(t, deployment) {
			for _, path := range [][]string{
				{"resources", "requests", "cpu"},
				{"resources", "requests", "memory"},
				{"resources", "limits", "cpu"},
				{"resources", "limits", "memory"},
			} {
				value, found, err := unstructured.NestedString(container, path...)
				require.NoError(t, err)
				assert.True(
					t, found && value != "",
					"%s container %s has no %v", deploymentName, containerName, path,
				)
			}
		}
	}
}

// TestComponentResourcesOverrideOneValueAndKeepTheRest covers an override that
// sets only some values: the rest keep their defaults rather than being written
// as empty quantities the API server would reject.
func TestComponentResourcesOverrideOneValueAndKeepTheRest(t *testing.T) {
	cpi := testInstaller()
	cpi.Opts.ComponentResources = map[string]ContainerResources{
		ThreeportRestApiName: {LimitMemory: "2Gi"},
	}

	defaults := DefaultComponentResources(ThreeportRestApiName)
	assert.Equal(t, map[string]interface{}{
		"requests": map[string]interface{}{"cpu": defaults.RequestCPU, "memory": defaults.RequestMemory},
		"limits":   map[string]interface{}{"cpu": defaults.LimitCPU, "memory": "2Gi"},
	}, cpi.componentResources(ThreeportRestApiName))

	// and only the component it names
	assert.Equal(t, DefaultComponentResources(ThreeportAgentName).manifest(), cpi.componentResources(ThreeportAgentName))
}

// TestComponentResourcesReachTheirContainer covers the wiring from an override
// to the container it names, for each kind of component, so an override cannot
// silently land on a different container or none.
func TestComponentResourcesReachTheirContainer(t *testing.T) {
	cpi := testInstaller()
	cpi.Opts.ComponentResources = map[string]ContainerResources{
		ThreeportRestApiName:             {LimitMemory: "901Mi"},
		ThreeportDatabaseMigratorName:    {LimitMemory: "902Mi"},
		ThreeportAgentName:               {LimitMemory: "903Mi"},
		ThreeportSecretControllerName:    {LimitMemory: "904Mi"},
		ThreeportTerraformControllerName: {LimitMemory: "905Mi"},
	}
	deployments := controlPlaneDeployments(t, cpi)

	for _, c := range []struct {
		deployment string
		container  string
		want       string
	}{
		{ThreeportAPIServiceResourceName, "api-server", "901Mi"},
		{ThreeportAPIServiceResourceName, "db-init", "902Mi"},
		{ThreeportAPIServiceResourceName, "database-migrator", "902Mi"},
		{ThreeportAgentDeployName, "manager", "903Mi"},
		{"threeport-" + ThreeportSecretControllerName, ThreeportSecretControllerName, "904Mi"},
		{"threeport-" + ThreeportTerraformControllerName, ThreeportTerraformControllerName, "905Mi"},
	} {
		container := podContainers(t, deployments[c.deployment])[c.container]
		require.NotNil(t, container, "%s has no container %s", c.deployment, c.container)
		got, _, err := unstructured.NestedString(container, "resources", "limits", "memory")
		require.NoError(t, err)
		assert.Equal(t, c.want, got, "%s container %s", c.deployment, c.container)
	}
}

// TestControllersThatRunHeavyWorkGetMoreMemory covers the controllers whose
// memory the shared default would not fit: the terraform controller runs the
// terraform binary and its providers in its container, the aws, oci and gcp
// controllers run pulumi and its provider plugins the same way, and the helm
// workload controller renders charts in process.
func TestControllersThatRunHeavyWorkGetMoreMemory(t *testing.T) {
	shared := resource.MustParse(DefaultComponentResources(ThreeportSecretControllerName).LimitMemory)
	for _, name := range []string{
		ThreeportTerraformControllerName,
		ThreeportAwsControllerName,
		ThreeportOciControllerName,
		ThreeportGcpControllerName,
		ThreeportHelmWorkloadControllerName,
	} {
		limit := resource.MustParse(DefaultComponentResources(name).LimitMemory)
		assert.Equal(t, 1, limit.Cmp(shared), "%s memory limit %s is not above the shared %s", name, limit.String(), shared.String())
	}
}

// TestDefaultComponentResourcesAreValid covers every default the API server
// would otherwise be the first to reject: an unparseable quantity, or a request
// above its limit, fails the Pod rather than the install.
func TestDefaultComponentResourcesAreValid(t *testing.T) {
	names := []string{ThreeportRestApiName, ThreeportAgentName, ThreeportDatabaseMigratorName}
	for _, controller := range ThreeportControllerList {
		names = append(names, controller.Name)
	}

	for _, name := range names {
		defaults := DefaultComponentResources(name)
		for _, pair := range [][2]string{
			{defaults.RequestCPU, defaults.LimitCPU},
			{defaults.RequestMemory, defaults.LimitMemory},
		} {
			request, err := resource.ParseQuantity(pair[0])
			require.NoError(t, err, "%s request %q", name, pair[0])
			limit, err := resource.ParseQuantity(pair[1])
			require.NoError(t, err, "%s limit %q", name, pair[1])
			assert.LessOrEqual(t, request.Cmp(limit), 0, "%s request %s is above its limit %s", name, pair[0], pair[1])
		}
	}
}
