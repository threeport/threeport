package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

var pdbGVR = schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}

// tierInstaller returns an installer for the given tier.
func tierInstaller(tier string) *ControlPlaneInstaller {
	cpi := testInstaller()
	cpi.Opts.Tier = ControlPlaneTier(tier)
	return cpi
}

// replicas returns a Deployment's replica count.
func replicas(t *testing.T, deployment *unstructured.Unstructured) int64 {
	t.Helper()

	count, found, err := unstructured.NestedInt64(deployment.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found, "%s sets no replicas", deployment.GetName())

	return count
}

// TestReplicasFollowTheTier covers the issue's own split: production runs the
// API server and controllers with a second replica to fail over to, while
// development keeps one, as does an install that records no tier at all.  The
// agent is not part of this and stays at one.
func TestReplicasFollowTheTier(t *testing.T) {
	for tier, want := range map[string]int64{
		ControlPlaneTierProd: productionComponentReplicas,
		ControlPlaneTierDev:  1,
		"":                   1,
	} {
		for name, deployment := range controlPlaneDeployments(t, tierInstaller(tier)) {
			if name == ThreeportAgentDeployName {
				assert.Equal(t, int64(1), replicas(t, deployment), "tier %q: agent", tier)
				continue
			}
			assert.Equal(t, want, replicas(t, deployment), "tier %q: %s", tier, name)
		}
	}
	assert.Greater(t, int64(productionComponentReplicas), int64(1))
}

// TestConfiguredReplicasOverrideTheTier covers the configurable half: an
// operator who sets a count gets it on either tier.
func TestConfiguredReplicasOverrideTheTier(t *testing.T) {
	for _, tier := range []string{ControlPlaneTierDev, ControlPlaneTierProd} {
		cpi := tierInstaller(tier)
		cpi.Opts.ApiReplicas = 3
		cpi.Opts.ControllerReplicas = 4

		for name, deployment := range controlPlaneDeployments(t, cpi) {
			switch name {
			case ThreeportAgentDeployName:
				assert.Equal(t, int64(1), replicas(t, deployment), "tier %q: agent", tier)
			case ThreeportAPIServiceResourceName:
				assert.Equal(t, int64(3), replicas(t, deployment), "tier %q: %s", tier, name)
			default:
				assert.Equal(t, int64(4), replicas(t, deployment), "tier %q: %s", tier, name)
			}
		}
	}
}

// installComponent installs the API server or one controller into kubeClient.
func installComponent(t *testing.T, cpi *ControlPlaneInstaller, kubeClient dynamic.Interface, deployName string) {
	t.Helper()

	mapper := testComponentMapper()
	if deployName == ThreeportAPIServiceResourceName {
		dbCreds, err := auth.GenerateDbCreds(cpi.Opts.Namespace)
		require.NoError(t, err)
		require.NoError(t, cpi.UpdateThreeportAPIDeployment(kubeClient, &mapper, dbCreds))
		return
	}

	for _, controller := range cpi.Opts.ControllerList {
		if "threeport-"+controller.Name == deployName {
			require.NoError(t, cpi.UpdateControllerDeployment(kubeClient, &mapper, *controller))
			return
		}
	}
	t.Fatalf("no component deploys %s", deployName)
}

// componentsWithBudgets is the API server and one controller, standing in for
// the two install paths a budget is applied on.
var componentsWithBudgets = []string{
	ThreeportAPIServiceResourceName,
	"threeport-" + ThreeportSecretControllerName,
}

// TestProductionComponentsGetADisruptionBudgetThatSelectsTheirPods covers the
// budget the issue asks for once there is more than one replica, and that it
// selects the Deployment's own pods - one that selected none would protect
// nothing and report no problem.
func TestProductionComponentsGetADisruptionBudgetThatSelectsTheirPods(t *testing.T) {
	for _, name := range componentsWithBudgets {
		kubeClient := testKubeClient()
		installComponent(t, tierInstaller(ControlPlaneTierProd), kubeClient, name)

		budget, err := kubeClient.Resource(pdbGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), name, metav1.GetOptions{})
		require.NoError(t, err, "%s has no disruption budget", name)

		minAvailable, _, err := unstructured.NestedInt64(budget.Object, "spec", "minAvailable")
		require.NoError(t, err)
		assert.Equal(t, int64(1), minAvailable, name)

		selector, _, err := unstructured.NestedStringMap(budget.Object, "spec", "selector", "matchLabels")
		require.NoError(t, err)
		podLabels, _, err := unstructured.NestedStringMap(
			getDeployment(t, kubeClient, ControlPlaneNamespace, name).Object,
			"spec", "template", "metadata", "labels",
		)
		require.NoError(t, err)
		require.NotEmpty(t, selector, name)
		for key, value := range selector {
			assert.Equal(t, value, podLabels[key], "%s budget selects %s=%s, which its pods do not carry", name, key, value)
		}
	}
}

// TestASingleReplicaComponentHasNoDisruptionBudget covers the budget that would
// be worse than none: one available pod out of one can never be met while that
// pod is evicted, so a node drain would wait on it forever.  A budget left by
// an earlier install with more replicas is removed rather than kept.
func TestASingleReplicaComponentHasNoDisruptionBudget(t *testing.T) {
	for _, name := range componentsWithBudgets {
		cpi := tierInstaller(ControlPlaneTierDev)
		kubeClient := testKubeClient(cpi.componentDisruptionBudget(name))
		installComponent(t, cpi, kubeClient, name)

		_, err := kubeClient.Resource(pdbGVR).Namespace(ControlPlaneNamespace).Get(context.Background(), name, metav1.GetOptions{})
		assert.True(t, k8serrors.IsNotFound(err), "%s keeps a disruption budget with one replica: %v", name, err)
	}
}

// TestReplicasPreferSeparateNodes covers what a second replica is for: two
// replicas on one node do not survive the loss of that node.  The rule has to
// select the Deployment's own pods to spread them.
func TestReplicasPreferSeparateNodes(t *testing.T) {
	for name, deployment := range controlPlaneDeployments(t, tierInstaller(ControlPlaneTierProd)) {
		if name == ThreeportAgentDeployName {
			continue
		}

		terms, found, err := unstructured.NestedSlice(
			deployment.Object, "spec", "template", "spec", "affinity", "podAntiAffinity",
			"preferredDuringSchedulingIgnoredDuringExecution",
		)
		require.NoError(t, err)
		require.True(t, found && len(terms) == 1, "%s has no anti-affinity", name)

		term := terms[0].(map[string]interface{})
		topology, _, _ := unstructured.NestedString(term, "podAffinityTerm", "topologyKey")
		assert.Equal(t, "kubernetes.io/hostname", topology, name)

		selector, _, _ := unstructured.NestedStringMap(term, "podAffinityTerm", "labelSelector", "matchLabels")
		podLabels, _, _ := unstructured.NestedStringMap(deployment.Object, "spec", "template", "metadata", "labels")
		require.NotEmpty(t, selector, name)
		for key, value := range selector {
			assert.Equal(t, value, podLabels[key], "%s anti-affinity selects %s=%s, which its pods do not carry", name, key, value)
		}
	}
}
