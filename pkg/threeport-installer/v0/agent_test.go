package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
)

// installerInNamespace returns an installer for the namespace.  NewInstaller
// applies its options to the package's shared defaults, so the default is put
// back when the test ends rather than left for the next test to inherit.
func installerInNamespace(t *testing.T, namespace string) *ControlPlaneInstaller {
	t.Helper()

	original := defaultInstallerOptions.Namespace
	t.Cleanup(func() { defaultInstallerOptions.Namespace = original })

	return NewInstaller(Namespace(namespace))
}

// two agents on one cluster must not address the same cluster-scoped objects
func TestAgentBindingNamesAreNamespaceQualified(t *testing.T) {
	first := installerInNamespace(t, "first")
	second := installerInNamespace(t, "second")

	assert.Equal(t, "first-threeport-agent-manager-rolebinding", first.agentManagerBindingName())
	assert.Equal(t, "first-threeport-agent-proxy-rolebinding", first.agentProxyBindingName())
	assert.NotEqual(t, first.agentManagerBindingName(), second.agentManagerBindingName())
	assert.NotEqual(t, first.agentProxyBindingName(), second.agentProxyBindingName())
}

// the compute space agent has no control plane name, and a label value cannot
// end in a hyphen
func TestAgentInstanceLabel(t *testing.T) {
	cpi := NewInstaller()

	cpi.Opts.ControlPlaneName = "my-control-plane"
	assert.Equal(t, "threeport-agent-my-control-plane", cpi.agentInstanceLabel())
	assert.Empty(t, validation.IsValidLabelValue(cpi.agentInstanceLabel()))

	cpi.Opts.ControlPlaneName = ""
	assert.Equal(t, "threeport-agent", cpi.agentInstanceLabel())
	assert.Empty(t, validation.IsValidLabelValue(cpi.agentInstanceLabel()))
}

func TestAgentArgsCarryControlPlaneName(t *testing.T) {
	cpi := NewInstaller()
	for _, arg := range cpi.getAgentArgs() {
		assert.NotContains(t, arg, "--control-plane-name")
	}

	cpi.Opts.ControlPlaneName = "my-control-plane"
	assert.Contains(t, cpi.getAgentArgs(), "--control-plane-name=my-control-plane")

	cpi.Opts.Debug = true
	assert.Contains(t, cpi.getAgentArgs(), "--control-plane-name=my-control-plane")
}

func TestControllerSecretCarriesControlPlaneName(t *testing.T) {
	cpi := NewInstaller()
	cpi.Opts.ControlPlaneName = "my-control-plane"
	cpi.Opts.RestApiInfo = ThreeportRestApi

	secret := cpi.getControllerSecret("controller", cpi.Opts.Namespace)
	data := secret.Object["stringData"].(map[string]interface{})
	assert.Equal(t, "my-control-plane", data["CONTROL_PLANE_NAME"])
}

// removing a control plane has to take its agent's cluster-scoped bindings
// with it, because deleting the namespace does not
func TestDeleteThreeportAgentBindings(t *testing.T) {
	removed := installerInNamespace(t, "removed")
	kept := installerInNamespace(t, "kept")

	client := testClient(append(agentBindings(removed), agentBindings(kept)...)...)
	mapper := testMapper()

	require.NoError(t, removed.DeleteThreeportAgentBindings(client, mapper))

	for _, name := range []string{removed.agentManagerBindingName(), removed.agentProxyBindingName()} {
		_, present, err := resourceInstalled(client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name)
		require.NoError(t, err)
		assert.False(t, present, "%s should be deleted", name)
	}
	for _, name := range []string{kept.agentManagerBindingName(), kept.agentProxyBindingName()} {
		_, present, err := resourceInstalled(client, mapper, "rbac.authorization.k8s.io", "v1", "ClusterRoleBinding", "", name)
		require.NoError(t, err)
		assert.True(t, present, "%s belongs to another control plane", name)
	}

	// a second pass finds nothing to delete and is not an error
	require.NoError(t, removed.DeleteThreeportAgentBindings(client, mapper))
}
