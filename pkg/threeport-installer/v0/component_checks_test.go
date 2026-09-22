package v0

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// These checks decide whether a live cluster gets written to, so what matters
// is that they say "already done" only when it really is: a check that answers
// true too readily leaves a broken cluster unrepaired, which is worse than the
// redundant installs they exist to avoid.

// testMapper maps the kinds the checks look up.
func testMapper() *meta.RESTMapper {
	defaultMapper := meta.NewDefaultRESTMapper(nil)
	defaultMapper.Add(
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		meta.RESTScopeNamespace,
	)
	defaultMapper.Add(
		schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"},
		meta.RESTScopeRoot,
	)
	defaultMapper.Add(
		schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
		meta.RESTScopeRoot,
	)

	var mapper meta.RESTMapper = defaultMapper

	return &mapper
}

// testClient returns a dynamic client holding the given objects.
func testClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:                               "DeploymentList",
		{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}: "CustomResourceDefinitionList",
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}:  "ClusterRoleBindingList",
	}

	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
}

// deployment builds a deployment carrying the given container images.
func deployment(namespace, name string, images ...string) *unstructured.Unstructured {
	containers := []interface{}{}
	for _, image := range images {
		containers = append(containers, map[string]interface{}{"image": image})
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": containers,
					},
				},
			},
		},
	}
}

// crd builds a custom resource definition with the given name.
func crd(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata":   map[string]interface{}{"name": name},
		},
	}
}

// clusterRoleBinding builds a binding with the given subject name.
func clusterRoleBinding(name, subject string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata":   map[string]interface{}{"name": name},
			"subjects": []interface{}{
				map[string]interface{}{"kind": "User", "name": subject},
			},
		},
	}
}

// agentImageFor returns the agent image an installer would install.
func agentImageFor(cpi *ControlPlaneInstaller) string {
	return cpi.getImage(
		cpi.Opts.AgentInfo.Name,
		cpi.Opts.AgentInfo.ImageName,
		cpi.Opts.AgentInfo.ImageNamespace,
		cpi.Opts.AgentInfo.ImageTag,
	)
}

// TestComputeSpaceControlPlaneComponentsCurrent covers the check that guards
// the compute space install and, with it, the ten second wait for the CRDs to
// settle.
func TestComputeSpaceControlPlaneComponentsCurrent(t *testing.T) {
	cpi := NewInstaller()
	installed := agentImageFor(cpi)

	t.Run("current when the agent runs the image that would be installed", func(t *testing.T) {
		client := testClient(
			deployment(cpi.Opts.Namespace, ThreeportAgentDeployName, "kube-rbac-proxy:v0.22.0", installed),
			crd(ThreeportCertManagerCRDName),
		)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.True(t, current)
	})

	t.Run("not current when the agent is absent", func(t *testing.T) {
		client := testClient(crd(ThreeportCertManagerCRDName))

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.False(t, current)
	})

	// an instance carrying a new ThreeportAgentImage needs the install to run
	// even though an agent is already there, so presence alone cannot be the
	// answer when the install can replace what is running
	t.Run("not current when an updating install would change the agent's image", func(t *testing.T) {
		updating := NewInstaller()
		updating.Opts.CreateOrUpdateKubeResources = true
		defer func() { updating.Opts.CreateOrUpdateKubeResources = false }()

		client := testClient(
			deployment(updating.Opts.Namespace, ThreeportAgentDeployName, "ghcr.io/threeport/threeport-agent:v0.0.1"),
			crd(ThreeportCertManagerCRDName),
		)

		current, err := updating.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.False(t, current)
	})

	// an install that only creates leaves a running agent alone whatever image
	// it carries, so reinstalling over a mismatch is work that cannot succeed -
	// and on a cluster installed from another registry it would be work done on
	// every invocation, which is what this whole change exists to stop
	t.Run("current despite a different image when the install only creates", func(t *testing.T) {
		require.False(t, cpi.Opts.CreateOrUpdateKubeResources)

		client := testClient(
			deployment(cpi.Opts.Namespace, ThreeportAgentDeployName, "localhost:5001/threeport-agent:v0.7.0-rc.0"),
			crd(ThreeportCertManagerCRDName),
		)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.True(t, current)
	})

	// the support services operator install that follows needs these
	// registered, so a missing CRD has to reinstall even with the agent in
	// place
	t.Run("not current when the CRDs are absent", func(t *testing.T) {
		client := testClient(
			deployment(cpi.Opts.Namespace, ThreeportAgentDeployName, installed),
		)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.False(t, current)
	})
}

// TestSupportServicesOperatorInstalled covers the operator check.
func TestSupportServicesOperatorInstalled(t *testing.T) {
	t.Run("installed when the deployment is there", func(t *testing.T) {
		client := testClient(
			deployment(ControlPlaneNamespace, SupportServicesOperatorDeployName, "operator:v1"),
		)

		installed, err := SupportServicesOperatorInstalled(client, testMapper())
		require.NoError(t, err)
		assert.True(t, installed)
	})

	t.Run("not installed on an empty cluster", func(t *testing.T) {
		installed, err := SupportServicesOperatorInstalled(testClient(), testMapper())
		require.NoError(t, err)
		assert.False(t, installed)
	})
}

// TestEksThreeportSystemServicesInstalled covers the EKS check. The install it
// guards only runs on EKS, but the check itself is ordinary API reads and is
// covered here rather than left to a cluster nobody can reach from a test.
func TestEksThreeportSystemServicesInstalled(t *testing.T) {
	t.Run("installed when the cluster autoscaler is there", func(t *testing.T) {
		client := testClient(
			deployment(ClusterAutoscalerNamespace, ClusterAutoscalerDeployName, "autoscaler:v1"),
		)

		installed, err := EksThreeportSystemServicesInstalled(client, testMapper())
		require.NoError(t, err)
		assert.True(t, installed)
	})

	t.Run("not installed on an empty cluster", func(t *testing.T) {
		installed, err := EksThreeportSystemServicesInstalled(testClient(), testMapper())
		require.NoError(t, err)
		assert.False(t, installed)
	})
}

// TestComputeSpaceWorkloadControllerRBACCurrent covers the GKE binding check.
func TestComputeSpaceWorkloadControllerRBACCurrent(t *testing.T) {
	cpi := NewInstaller()
	const project = "a-gcp-project"

	subjectFor := func(controllerName string) string {
		return fmt.Sprintf(
			"serviceAccount:%s.svc.id.goog[%s/%s]",
			project, cpi.Opts.Namespace, controllerName,
		)
	}

	allBindings := func() []runtime.Object {
		var objects []runtime.Object
		for _, controllerName := range computeSpaceWorkloadControllers() {
			objects = append(objects, clusterRoleBinding(
				fmt.Sprintf("%s-cluster-admin", controllerName),
				subjectFor(controllerName),
			))
		}

		return objects
	}

	t.Run("current when every binding names this project", func(t *testing.T) {
		current, err := cpi.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(allBindings()...), testMapper(), project,
		)
		require.NoError(t, err)
		assert.True(t, current)
	})

	t.Run("not current when one binding is missing", func(t *testing.T) {
		bindings := allBindings()
		require.Greater(t, len(bindings), 1)

		current, err := cpi.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(bindings[1:]...), testMapper(), project,
		)
		require.NoError(t, err)
		assert.False(t, current)
	})

	// a binding left from another project authorizes a principal that no longer
	// exists, so the controllers it was meant to authorize cannot reach the
	// cluster - present is not the same as current
	t.Run("not current when a binding names another project", func(t *testing.T) {
		current, err := cpi.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(allBindings()...), testMapper(), "a-different-project",
		)
		require.NoError(t, err)
		assert.False(t, current)
	})
}

// TestResourceInstalledDoesNotReadFailureAsAbsence covers the distinction the
// checks rest on. Reading an API error as "not there" would reinstall against a
// cluster that needs nothing, which is the behaviour these checks exist to stop.
func TestResourceInstalledDoesNotReadFailureAsAbsence(t *testing.T) {
	client := testClient()
	client.PrependReactor("get", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("the API server is unreachable")
	})

	_, present, err := resourceInstalled(
		client, testMapper(), "apps", "v1", "Deployment", ControlPlaneNamespace, SupportServicesOperatorDeployName,
	)
	require.Error(t, err)
	assert.False(t, present)
}

// TestResourceInstalledReadsAnUnregisteredKindAsAbsent covers a cluster whose
// CRDs were never installed: the kind cannot be mapped at all, which is an
// answer rather than a failure.
func TestResourceInstalledReadsAnUnregisteredKindAsAbsent(t *testing.T) {
	emptyMapper := meta.NewDefaultRESTMapper(nil)
	var mapper meta.RESTMapper = emptyMapper

	_, present, err := resourceInstalled(
		testClient(), &mapper, "apiextensions.k8s.io", "v1", "CustomResourceDefinition", "", ThreeportCertManagerCRDName,
	)
	require.NoError(t, err)
	assert.False(t, present)
}
