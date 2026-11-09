package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/validation"
)

// two agents on one cluster must not address the same cluster-scoped objects
func TestAgentBindingNamesAreNamespaceQualified(t *testing.T) {
	first := NewInstaller(Namespace("first"))
	second := NewInstaller(Namespace("second"))

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
