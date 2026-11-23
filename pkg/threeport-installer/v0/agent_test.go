package v0

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	secret := cpi.getControllerSecret("controller", cpi.Opts.Namespace, &natsCredentials{User: NatsUser, Password: "tpSecret"}, 1)
	data := secret.Object["stringData"].(map[string]interface{})
	assert.Equal(t, "my-control-plane", data["CONTROL_PLANE_NAME"])
	_, replaced := data[ControllerConfigReplacedControlPlaneKey]
	assert.False(t, replaced)

	cpi.Opts.SourceName = "old-control-plane"
	secret = cpi.getControllerSecret("controller", cpi.Opts.Namespace, &natsCredentials{User: NatsUser, Password: "tpSecret"}, 1)
	data = secret.Object["stringData"].(map[string]interface{})
	assert.Equal(t, "old-control-plane", data[ControllerConfigReplacedControlPlaneKey])
}

// TestTheControlPlanesAgentChecksItsMessageBroker covers the agent on the
// control plane's own cluster checking the control plane's message broker's
// storage, and recording the warning as an event, which it may: an agent on
// a managed runtime, which reports across the network and has no broker
// beside it, checks none.
func TestTheControlPlanesAgentChecksItsMessageBroker(t *testing.T) {
	cpi := componentsTestInstaller()
	assert.Contains(t, cpi.getAgentArgs(), "--message-broker-service=nats-js")
	assert.Contains(t, cpi.getAgentArgs(), "--message-broker-namespace="+ControlPlaneNamespace)

	remote := componentsTestInstaller()
	remote.Opts.AgentApiServer = "https://threeport.example.com"
	for _, arg := range remote.getAgentArgs() {
		assert.NotContains(t, arg, "--message-broker", "an agent on a managed runtime")
	}

	kubeClient := testKubeClient()
	mapper := testComponentMapper()
	require.NoError(t, cpi.UpdateThreeportAgentDeployment(kubeClient, &mapper))
	role, err := kubeClient.Resource(schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}).
		Namespace(ControlPlaneNamespace).Get(context.Background(), "threeport-agent-leader-election-role", metav1.GetOptions{})
	require.NoError(t, err)
	rules, _, _ := unstructured.NestedSlice(role.Object, "rules")
	recordsEvents := false
	for _, rule := range rules {
		groups, _, _ := unstructured.NestedStringSlice(rule.(map[string]interface{}), "apiGroups")
		resources, _, _ := unstructured.NestedStringSlice(rule.(map[string]interface{}), "resources")
		verbs, _, _ := unstructured.NestedStringSlice(rule.(map[string]interface{}), "verbs")
		if slices.Contains(groups, "events.k8s.io") && slices.Contains(resources, "events") && slices.Contains(verbs, "create") {
			recordsEvents = true
		}
	}
	assert.True(t, recordsEvents, "the agent records events through events.k8s.io")
}
