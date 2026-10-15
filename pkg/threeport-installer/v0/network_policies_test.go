package v0

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

func testInstaller(controlPlaneName string, controllers ...string) *ControlPlaneInstaller {
	installer := &ControlPlaneInstaller{}
	installer.Opts.Namespace = "threeport-control-plane"
	installer.Opts.Name = controlPlaneName
	for _, name := range controllers {
		installer.Opts.ControllerList = append(
			installer.Opts.ControllerList,
			&v0.ControlPlaneComponent{Name: name},
		)
	}

	return installer
}

// policyNamed returns one policy by name, and the names of all of them.
func policyNamed(t *testing.T, policies []*unstructured.Unstructured, name string) *unstructured.Unstructured {
	t.Helper()
	var names []string
	for _, policy := range policies {
		names = append(names, policy.GetName())
		if policy.GetName() == name {
			return policy
		}
	}
	require.FailNowf(t, "policy not found", "%s is not among %v", name, names)

	return nil
}

// selectedNames returns the app.kubernetes.io/name values a selector matches.
func selectedNames(t *testing.T, selector map[string]interface{}) []string {
	t.Helper()
	expressions, ok, err := unstructured.NestedSlice(selector, "matchExpressions")
	require.NoError(t, err)
	if !ok {
		return nil
	}
	var values []string
	for _, expression := range expressions {
		list, _, err := unstructured.NestedStringSlice(expression.(map[string]interface{}), "values")
		require.NoError(t, err)
		values = append(values, list...)
	}

	return values
}

// TestDefaultDenyIsPresent covers the policy the others depend on.
//
// A NetworkPolicy only grants: a pod no policy selects is unrestricted, so
// without one selecting every pod in both directions, the allow rules describe
// paths that were already open and the whole set is decoration.
func TestDefaultDenyIsPresent(t *testing.T) {
	policies := testInstaller("threeport", "kubernetes-workload-controller").threeportNetworkPolicies()

	denyAll := policyNamed(t, policies, "threeport-default-deny")

	selector, _, err := unstructured.NestedMap(denyAll.Object, "spec", "podSelector")
	require.NoError(t, err)
	assert.Empty(t, selector, "the default deny has to select every pod")

	types, _, err := unstructured.NestedStringSlice(denyAll.Object, "spec", "policyTypes")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Ingress", "Egress"}, types)

	for _, direction := range []string{"ingress", "egress"} {
		rules, found, err := unstructured.NestedSlice(denyAll.Object, "spec", direction)
		require.NoError(t, err)
		assert.False(t, found && len(rules) > 0, "a rule in the default deny would grant %s", direction)
	}

	assert.Equal(t, "threeport-default-deny", policies[0].GetName(), "the deny is applied first")
}

// TestControllerSelectorsFollowTheDeploymentNames covers the link between a
// policy and the pods it is meant to select.
//
// A controller's pod label is its deployment name, and that name is not the
// controller's name: a module's controller is prefixed with the control plane
// it belongs to, while the built-in ones keep the threeport prefix on every
// control plane. A policy spelling either form out would select nothing for
// the other - and selecting nothing is not an error, it is a policy that
// silently denies the traffic it was written to allow.
func TestControllerSelectorsFollowTheDeploymentNames(t *testing.T) {
	const moduleController = "springboot-controller"

	for _, testCase := range []struct {
		name             string
		controlPlaneName string
		controller       string
		expected         string
	}{
		{
			name:             "a built-in controller keeps the threeport prefix",
			controlPlaneName: "child-one",
			controller:       ThreeportKubernetesWorkloadControllerName,
			expected:         "threeport-" + ThreeportKubernetesWorkloadControllerName,
		},
		{
			name:             "a module's controller takes the control plane's name",
			controlPlaneName: "child-one",
			controller:       moduleController,
			expected:         "child-one-" + moduleController,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			installer := testInstaller(testCase.controlPlaneName, testCase.controller)
			policies := installer.threeportNetworkPolicies()

			controllerPolicy := policyNamed(t, policies, "threeport-controllers")
			selector, _, err := unstructured.NestedMap(controllerPolicy.Object, "spec", "podSelector")
			require.NoError(t, err)

			assert.Contains(t, selectedNames(t, selector), testCase.expected)
		})
	}
}

// TestEveryInstalledControllerReachesWhatItNeeds covers the gap between the
// controllers an install deploys and the ones its policies name.
//
// The set is chosen per install, so a policy built from a fixed list would
// leave whichever controllers were added since unable to reach the API server
// or the message broker - and a denied controller does not fail at install, it
// fails later, intermittently, while reconciling.
func TestEveryInstalledControllerReachesWhatItNeeds(t *testing.T) {
	controllers := []string{
		"kubernetes-workload-controller",
		"helm-workload-controller",
		"control-plane-controller",
		"gateway-controller",
	}
	installer := testInstaller("threeport", controllers...)
	policies := installer.threeportNetworkPolicies()

	selectedByControllerPolicy := func(policyName string) []string {
		policy := policyNamed(t, policies, policyName)
		selector, _, err := unstructured.NestedMap(policy.Object, "spec", "podSelector")
		require.NoError(t, err)

		return selectedNames(t, selector)
	}

	egressSelected := selectedByControllerPolicy("threeport-controllers")
	for _, controller := range controllers {
		assert.Contains(t, egressSelected, "threeport-"+controller)
	}
	assert.Contains(t, egressSelected, ThreeportAgentDeployName, "the agent reaches the API server too")

	// and the broker has to accept them
	broker := policyNamed(t, policies, "threeport-message-broker")
	ingress, _, err := unstructured.NestedSlice(broker.Object, "spec", "ingress")
	require.NoError(t, err)
	from, _, err := unstructured.NestedSlice(ingress[0].(map[string]interface{}), "from")
	require.NoError(t, err)
	selector, _, err := unstructured.NestedMap(from[0].(map[string]interface{}), "podSelector")
	require.NoError(t, err)
	brokerClients := selectedNames(t, selector)
	for _, controller := range controllers {
		assert.Contains(t, brokerClients, "threeport-"+controller)
	}
	assert.Contains(t, brokerClients, ThreeportAPIServiceResourceName)
}

// TestOnlyTheApiServerReachesTheDatabase covers the restriction that carries
// most of the value here.
//
// Three controllers hold cluster-admin on the cluster they run in, and every
// component reads through the API server rather than the database. So the
// database accepting anything else is a path that exists for no caller.
func TestOnlyTheApiServerReachesTheDatabase(t *testing.T) {
	policies := testInstaller("threeport", "kubernetes-workload-controller").threeportNetworkPolicies()

	database := policyNamed(t, policies, "threeport-database")
	ingress, _, err := unstructured.NestedSlice(database.Object, "spec", "ingress")
	require.NoError(t, err)
	from, _, err := unstructured.NestedSlice(ingress[0].(map[string]interface{}), "from")
	require.NoError(t, err)

	var allowed []string
	for _, source := range from {
		selector, _, err := unstructured.NestedMap(source.(map[string]interface{}), "podSelector")
		require.NoError(t, err)
		allowed = append(allowed, selectedNames(t, selector)...)
	}

	assert.ElementsMatch(
		t, []string{ThreeportAPIServiceResourceName, "cockroachdb"}, allowed,
		"only the API server and the database's own nodes reach the database",
	)
	assert.NotContains(t, allowed, "threeport-kubernetes-workload-controller")
}

// TestEveryWorkloadInTheNamespaceIsCovered covers the pods a policy set is
// easy to write without noticing.
//
// The default deny applies to the namespace, not to the components these
// policies were written for. Anything else installed there - the support
// services operator, the broker's utility pod - carries none of the labels the
// component policies select on, so it loses its network entirely. That failure
// does not look like a networking one: the operator reconciles the services a
// gateway depends on, so it shows up as gateways never becoming ready.
//
// The labels here are the ones those workloads are installed with, so a change
// to either side has to be made on both.
func TestEveryWorkloadInTheNamespaceIsCovered(t *testing.T) {
	installer := testInstaller("threeport", ThreeportControlPlaneControllerName)
	policies := installer.threeportNetworkPolicies()

	for _, workload := range []struct {
		name   string
		labels map[string]string
	}{
		{
			name:   "the API server",
			labels: map[string]string{"app.kubernetes.io/name": ThreeportAPIServiceResourceName},
		},
		{
			name:   "the database",
			labels: map[string]string{"app.kubernetes.io/name": "cockroachdb"},
		},
		{
			name:   "the message broker",
			labels: map[string]string{"app.kubernetes.io/name": "nats"},
		},
		{
			name:   "a controller",
			labels: map[string]string{"app.kubernetes.io/name": "threeport-" + ThreeportControlPlaneControllerName},
		},
		{
			name:   "the agent",
			labels: map[string]string{"app.kubernetes.io/name": ThreeportAgentDeployName},
		},
		{
			name:   "the support services operator",
			labels: map[string]string{"control-plane": "controller-manager"},
		},
		{
			name:   "the message broker's utility pod",
			labels: map[string]string{"app": "nats-js-box"},
		},
	} {
		t.Run(workload.name, func(t *testing.T) {
			var granting []string
			for _, policy := range policies {
				name := policy.GetName()
				if name == "threeport-default-deny" || name == "threeport-allow-dns" {
					continue
				}
				selector, _, err := unstructured.NestedMap(policy.Object, "spec", "podSelector")
				require.NoError(t, err)
				if selectorMatches(t, selector, workload.labels) {
					granting = append(granting, name)
				}
			}

			assert.NotEmpty(
				t, granting,
				"nothing but the default deny applies to this pod, so it has no network at all",
			)
		})
	}
}

// selectorMatches reports whether a pod selector matches a set of labels.
func selectorMatches(t *testing.T, selector map[string]interface{}, labels map[string]string) bool {
	t.Helper()

	matchLabels, _, err := unstructured.NestedStringMap(selector, "matchLabels")
	require.NoError(t, err)
	for key, value := range matchLabels {
		if labels[key] != value {
			return false
		}
	}

	expressions, found, err := unstructured.NestedSlice(selector, "matchExpressions")
	require.NoError(t, err)
	if !found {
		return len(matchLabels) > 0
	}
	for _, raw := range expressions {
		expression := raw.(map[string]interface{})
		key, _, err := unstructured.NestedString(expression, "key")
		require.NoError(t, err)
		values, _, err := unstructured.NestedStringSlice(expression, "values")
		require.NoError(t, err)
		if !slices.Contains(values, labels[key]) {
			return false
		}
	}

	return true
}
