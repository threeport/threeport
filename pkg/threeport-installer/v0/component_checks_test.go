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

	apiv0 "github.com/threeport/threeport/pkg/api/v0"
)

// These checks decide whether a live cluster gets written to, so what matters
// is that they say "already done" only when it really is: a check that answers
// true too readily leaves a broken cluster unrepaired, which is worse than the
// redundant installs they exist to avoid.

// testMapper maps the kinds the checks look up.
func testMapper() *meta.RESTMapper {
	// pass preferred versions so a versionless lookup resolves
	defaultMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "apps", Version: "v1"},
		{Group: "apiextensions.k8s.io", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
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

// installedCRDs returns every CRD the compute space install creates, as a
// cluster that install completed on holds them.
func installedCRDs() []runtime.Object {
	var objects []runtime.Object
	for _, name := range ThreeportCRDNames() {
		objects = append(objects, crd(name))
	}

	return objects
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

// clusterRoleBinding builds a binding granting one subject the cluster-admin
// role, as the installer would.
func clusterRoleBinding(name, subject string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata":   map[string]interface{}{"name": name},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "cluster-admin",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":     "User",
					"name":     subject,
					"apiGroup": "rbac.authorization.k8s.io",
				},
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
		client := testClient(append(
			[]runtime.Object{deployment(cpi.Opts.Namespace, ThreeportAgentDeployName, "kube-rbac-proxy:v0.22.0", installed)},
			installedCRDs()...,
		)...)

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

		client := testClient(append(
			[]runtime.Object{deployment(updating.Opts.Namespace, ThreeportAgentDeployName, "ghcr.io/threeport/threeport-agent:v0.0.1")},
			installedCRDs()...,
		)...)

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

		client := testClient(append(
			[]runtime.Object{deployment(cpi.Opts.Namespace, ThreeportAgentDeployName, "localhost:5001/threeport-agent:v0.7.0-rc.0")},
			installedCRDs()...,
		)...)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.True(t, current)
	})

	// TestComputeSpaceControlPlaneComponentsCurrent covers a cluster holding only
	// some of the CRDs the install creates.
	//
	// The support services operator install that follows registers against them,
	// so any one missing leaves it nothing to register against. Checking a single
	// CRD would call such a cluster ready and skip the install that would put the
	// rest there.
	t.Run("not current when any one CRD is missing", func(t *testing.T) {
		for _, missing := range ThreeportCRDNames() {
			present := []runtime.Object{
				deployment(cpi.Opts.Namespace, ThreeportAgentDeployName, installed),
			}
			for _, name := range ThreeportCRDNames() {
				if name != missing {
					present = append(present, crd(name))
				}
			}

			current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(testClient(present...), testMapper())
			require.NoError(t, err)
			assert.False(t, current, "%s is missing, so the set is not current", missing)
		}
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

	// deliberately not cpi.Opts.Namespace: the principal names where the
	// control plane's controllers run, and the installer's namespace is where
	// components go on the managed cluster. Using the same string for both
	// would let the two be confused without any test noticing.
	const controlPlaneNS = "a-custom-control-plane-namespace"

	subjectFor := func(controllerName string) string {
		return fmt.Sprintf(
			"serviceAccount:%s.svc.id.goog[%s/%s]",
			project, controlPlaneNS, controllerName,
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
			testClient(allBindings()...), testMapper(), project, "", controlPlaneNS,
		)
		require.NoError(t, err)
		assert.True(t, current)
	})

	t.Run("not current when one binding is missing", func(t *testing.T) {
		bindings := allBindings()
		require.Greater(t, len(bindings), 1)

		current, err := cpi.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(bindings[1:]...), testMapper(), project, "", controlPlaneNS,
		)
		require.NoError(t, err)
		assert.False(t, current)
	})

	// an installer that can update is able to replace a binding left from
	// another project, so the difference is worth reporting
	t.Run("not current when an updating install would fix another project's binding", func(t *testing.T) {
		updating := NewInstaller()
		updating.Opts.CreateOrUpdateKubeResources = true
		defer func() { updating.Opts.CreateOrUpdateKubeResources = false }()

		current, err := updating.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(allBindings()...), testMapper(), "a-different-project", "", controlPlaneNS,
		)
		require.NoError(t, err)
		assert.False(t, current)
	})

	// a create-only install cannot touch a binding that exists, so calling a
	// stale one not current would ask for a repair that can never land - on
	// every invocation, which is the waste this check exists to remove
	t.Run("current despite another project's binding when the install only creates", func(t *testing.T) {
		require.False(t, cpi.Opts.CreateOrUpdateKubeResources)

		current, err := cpi.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(allBindings()...), testMapper(), "a-different-project", "", controlPlaneNS,
		)
		require.NoError(t, err)
		assert.True(t, current)
	})

	// the subject's name alone is not enough: a binding can carry the right
	// name as the wrong kind of subject, and the controller still has no access
	t.Run("not current when a subject has the right name but the wrong kind", func(t *testing.T) {
		updating := NewInstaller()
		updating.Opts.CreateOrUpdateKubeResources = true
		defer func() { updating.Opts.CreateOrUpdateKubeResources = false }()

		var objects []runtime.Object
		for _, controllerName := range computeSpaceWorkloadControllers() {
			binding := clusterRoleBinding(
				fmt.Sprintf("%s-cluster-admin", controllerName), subjectFor(controllerName),
			)
			subjects, _, err := unstructured.NestedSlice(binding.Object, "subjects")
			require.NoError(t, err)
			subjects[0].(map[string]interface{})["kind"] = "ServiceAccount"
			require.NoError(t, unstructured.SetNestedSlice(binding.Object, subjects, "subjects"))
			objects = append(objects, binding)
		}

		current, err := updating.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(objects...), testMapper(), project, "", controlPlaneNS,
		)
		require.NoError(t, err)
		assert.False(t, current)
	})

	// nor is the subject enough on its own: a binding pointing at a different
	// role grants that role, not the one the controller needs
	t.Run("not current when a binding points at another role", func(t *testing.T) {
		updating := NewInstaller()
		updating.Opts.CreateOrUpdateKubeResources = true
		defer func() { updating.Opts.CreateOrUpdateKubeResources = false }()

		var objects []runtime.Object
		for _, controllerName := range computeSpaceWorkloadControllers() {
			binding := clusterRoleBinding(
				fmt.Sprintf("%s-cluster-admin", controllerName), subjectFor(controllerName),
			)
			require.NoError(t, unstructured.SetNestedField(
				binding.Object, "view", "roleRef", "name",
			))
			objects = append(objects, binding)
		}

		current, err := updating.ComputeSpaceWorkloadControllerRBACCurrent(
			testClient(objects...), testMapper(), project, "", controlPlaneNS,
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

// TestReconcileControllerSetDeletesDroppedDeployment deletes a catalog controller the selection dropped.
func TestReconcileControllerSetDeletesDroppedDeployment(t *testing.T) {
	require.GreaterOrEqual(t, len(ThreeportControllerList), 2)

	// select the first catalog controller and leave the second installed
	cpi := NewInstaller()
	cpi.Opts.ControllerList = ThreeportControllerList[:1]
	keptName := cpi.controllerDeploymentName(*ThreeportControllerList[0])
	droppedName := cpi.controllerDeploymentName(*ThreeportControllerList[1])
	client := testClient(
		deployment(cpi.Opts.Namespace, keptName),
		deployment(cpi.Opts.Namespace, droppedName),
	)
	mapper := testMapper()

	// delete the controller the selection dropped and leave the selected one
	require.NoError(t, cpi.ReconcileControllerSet(client, mapper, nil, false))

	_, droppedPresent, err := resourceInstalled(
		client, mapper, "apps", "v1", "Deployment", cpi.Opts.Namespace, droppedName,
	)
	require.NoError(t, err)
	assert.False(t, droppedPresent)

	_, keptPresent, err := resourceInstalled(
		client, mapper, "apps", "v1", "Deployment", cpi.Opts.Namespace, keptName,
	)
	require.NoError(t, err)
	assert.True(t, keptPresent)
}

// TestReconcileControllerSetKeepsSelfControlPlaneController checks the self controller stays installed.
func TestReconcileControllerSetKeepsSelfControlPlaneController(t *testing.T) {
	// pick the self controller and two others from the catalog
	var selected *apiv0.ControlPlaneComponent
	var dropped *apiv0.ControlPlaneComponent
	var selfController *apiv0.ControlPlaneComponent
	for _, controller := range ThreeportControllerList {
		if controller.Name == ThreeportControlPlaneControllerName {
			selfController = controller
			continue
		}
		if selected == nil {
			selected = controller
			continue
		}
		if dropped == nil {
			dropped = controller
		}
	}
	require.NotNil(t, selected)
	require.NotNil(t, dropped)
	require.NotNil(t, selfController)

	// leave the selected controller installed and drop another
	cpi := NewInstaller()
	cpi.Opts.ControllerList = []*apiv0.ControlPlaneComponent{selected}
	selectedName := cpi.controllerDeploymentName(*selected)
	keptName := cpi.controllerDeploymentName(*selfController)
	droppedName := cpi.controllerDeploymentName(*dropped)
	client := testClient(
		deployment(cpi.Opts.Namespace, selectedName),
		deployment(cpi.Opts.Namespace, keptName),
		deployment(cpi.Opts.Namespace, droppedName),
	)
	mapper := testMapper()

	// reconcile with the self controller retained
	require.NoError(t, cpi.ReconcileControllerSet(client, mapper, nil, true))

	// confirm the self controller stayed and the other dropped controller did not
	_, keptPresent, err := resourceInstalled(
		client, mapper, "apps", "v1", "Deployment", cpi.Opts.Namespace, keptName,
	)
	require.NoError(t, err)
	assert.True(t, keptPresent)
	_, droppedPresent, err := resourceInstalled(
		client, mapper, "apps", "v1", "Deployment", cpi.Opts.Namespace, droppedName,
	)
	require.NoError(t, err)
	assert.False(t, droppedPresent)
}

// TestDeleteAgentRoleBindingsLeavesOtherControlPlanes deletes one control plane's agent bindings and leaves the other's.
func TestDeleteAgentRoleBindingsLeavesOtherControlPlanes(t *testing.T) {
	// install agent bindings for two control planes
	cpi := NewInstaller()
	cpi.Opts.Namespace = "child"
	client := testClient(
		clusterRoleBinding("child-threeport-agent-manager-rolebinding", "agent"),
		clusterRoleBinding("child-threeport-agent-proxy-rolebinding", "agent"),
		clusterRoleBinding("threeport-control-plane-threeport-agent-manager-rolebinding", "agent"),
		clusterRoleBinding("threeport-control-plane-threeport-agent-proxy-rolebinding", "agent"),
	)
	mapper := testMapper()

	// delete the child bindings, then again once they are already gone
	require.NoError(t, cpi.DeleteAgentRoleBindings(client, mapper))
	require.NoError(t, cpi.DeleteAgentRoleBindings(client, mapper))

	// confirm the child bindings are gone
	for _, name := range []string{
		"child-threeport-agent-manager-rolebinding",
		"child-threeport-agent-proxy-rolebinding",
	} {
		_, present, err := resourceInstalled(
			client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name,
		)
		require.NoError(t, err)
		assert.False(t, present)
	}

	// confirm the other control plane's bindings stayed
	for _, name := range []string{
		"threeport-control-plane-threeport-agent-manager-rolebinding",
		"threeport-control-plane-threeport-agent-proxy-rolebinding",
	} {
		_, present, err := resourceInstalled(
			client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name,
		)
		require.NoError(t, err)
		assert.True(t, present)
	}
}

// TestDeleteControlPlaneRoleBindingsLeavesOtherControlPlanes deletes one control plane's role bindings and leaves the other's.
func TestDeleteControlPlaneRoleBindingsLeavesOtherControlPlanes(t *testing.T) {
	// install this control plane's bindings, another control plane's copies, and one unrelated binding
	cpi := NewInstaller()
	cpi.Opts.Namespace = "child"
	objects := []runtime.Object{
		clusterRoleBinding("child-threeport-agent-manager-rolebinding", "agent"),
		clusterRoleBinding("child-threeport-agent-proxy-rolebinding", "agent"),
		clusterRoleBinding("threeport-control-plane-threeport-agent-manager-rolebinding", "agent"),
		clusterRoleBinding("threeport-control-plane-threeport-agent-proxy-rolebinding", "agent"),
		clusterRoleBinding("cluster-autoscaler", "cluster-autoscaler"),
	}
	for _, controller := range ThreeportControllerList {
		if controller.ServiceAccountName == "" {
			continue
		}
		account := controller.ServiceAccountName
		for _, namespace := range []string{"child", "threeport-control-plane"} {
			for _, suffix := range []string{"threeportworkloads", "cluster-admin"} {
				name := fmt.Sprintf("%s-%s-%s", namespace, account, suffix)
				objects = append(objects, clusterRoleBinding(name, account))
			}
		}
	}
	client := testClient(objects...)
	mapper := testMapper()

	// delete the child bindings, then again once they are already gone
	require.NoError(t, cpi.DeleteControlPlaneRoleBindings(client, mapper))
	require.NoError(t, cpi.DeleteControlPlaneRoleBindings(client, mapper))

	// confirm the child agent bindings are gone
	for _, name := range []string{
		"child-threeport-agent-manager-rolebinding",
		"child-threeport-agent-proxy-rolebinding",
	} {
		_, present, err := resourceInstalled(
			client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name,
		)
		require.NoError(t, err)
		assert.False(t, present)
	}

	// confirm the child controller bindings are gone
	for _, controller := range ThreeportControllerList {
		if controller.ServiceAccountName == "" {
			continue
		}
		for _, suffix := range []string{"threeportworkloads", "cluster-admin"} {
			name := fmt.Sprintf("child-%s-%s", controller.ServiceAccountName, suffix)
			_, present, err := resourceInstalled(
				client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name,
			)
			require.NoError(t, err)
			assert.False(t, present, name)
		}
	}

	// confirm the other control plane's agent bindings stayed
	for _, name := range []string{
		"threeport-control-plane-threeport-agent-manager-rolebinding",
		"threeport-control-plane-threeport-agent-proxy-rolebinding",
	} {
		_, present, err := resourceInstalled(
			client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name,
		)
		require.NoError(t, err)
		assert.True(t, present)
	}

	// confirm the other control plane's controller bindings stayed
	for _, controller := range ThreeportControllerList {
		if controller.ServiceAccountName == "" {
			continue
		}
		for _, suffix := range []string{"threeportworkloads", "cluster-admin"} {
			name := fmt.Sprintf("threeport-control-plane-%s-%s", controller.ServiceAccountName, suffix)
			_, present, err := resourceInstalled(
				client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name,
			)
			require.NoError(t, err)
			assert.True(t, present, name)
		}
	}

	// confirm the unrelated binding stayed
	_, present, err := resourceInstalled(
		client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", "cluster-autoscaler",
	)
	require.NoError(t, err)
	assert.True(t, present)
}
