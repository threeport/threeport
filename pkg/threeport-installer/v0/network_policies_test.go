package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
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

// The pods a control plane namespace holds, and the ones beside it. The last
// three are what two earlier versions of this file got wrong: they arrive
// after the install, carry labels of their own, or live in another namespace
// altogether.
var (
	apiServerPod   = map[string]string{"app.kubernetes.io/name": ThreeportAPIServiceResourceName}
	databasePod    = map[string]string{"app.kubernetes.io/name": "cockroachdb"}
	brokerPod      = map[string]string{"app.kubernetes.io/name": "nats"}
	controllerPod  = map[string]string{"app.kubernetes.io/name": "threeport-kubernetes-workload-controller"}
	brokerUtility  = map[string]string{"app": "nats-js-box"}
	supportStack   = map[string]string{"control-plane": "controller-manager"}
	maintenanceJob = map[string]string{LabelMaintenance: "true"}
	modulePod      = map[string]string{"app.kubernetes.io/name": "threeport-test-api-server"}
)

// selectorMatches reports whether a pod selector matches a pod's labels.
func selectorMatches(t *testing.T, selector map[string]interface{}, labels map[string]string) bool {
	t.Helper()
	if len(selector) == 0 {
		return true
	}

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
		return true
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

// admits reports whether the policy lets a pod open a connection on a port.
func admits(t *testing.T, policy *unstructured.Unstructured, pod map[string]string, port int) bool {
	t.Helper()

	rules, found, err := unstructured.NestedSlice(policy.Object, "spec", "ingress")
	require.NoError(t, err)
	require.True(t, found, "the policy carries no ingress rule, so it admits nothing")

	for _, rawRule := range rules {
		rule := rawRule.(map[string]interface{})

		ports, _, err := unstructured.NestedSlice(rule, "ports")
		require.NoError(t, err)
		onPort := false
		for _, rawPort := range ports {
			number, _, err := unstructured.NestedInt64(rawPort.(map[string]interface{}), "port")
			require.NoError(t, err)
			if number == int64(port) {
				onPort = true
				break
			}
		}
		if !onPort {
			continue
		}

		peers, _, err := unstructured.NestedSlice(rule, "from")
		require.NoError(t, err)
		for _, rawPeer := range peers {
			selector, found, err := unstructured.NestedMap(rawPeer.(map[string]interface{}), "podSelector")
			require.NoError(t, err)
			if found && selectorMatches(t, selector, pod) {
				return true
			}
		}
	}

	return false
}

// TestOnlyTheApiServerReachesTheDatabase covers the restriction this exists
// for.
//
// The controllers hold cluster-admin on the cluster they run in, and none of
// them talks to the database - they go through the API server. So a foothold
// in any of them has no path to the credentials and state the database holds.
func TestOnlyTheApiServerReachesTheDatabase(t *testing.T) {
	policy := testInstaller("dev-0", "kubernetes-workload").databasePolicy()

	assert.True(
		t, admits(t, policy, apiServerPod, cockroachSqlPort),
		"the API server is the component that reads the database",
	)
	assert.True(
		t, admits(t, policy, databasePod, cockroachSqlPort),
		"the database nodes replicate between themselves",
	)
	assert.True(
		t, admits(t, policy, maintenanceJob, cockroachSqlPort),
		"a drop runs while the API server is scaled to zero, so it reaches the database itself",
	)

	for name, pod := range map[string]map[string]string{
		"a controller":                  controllerPod,
		"the message broker":            brokerPod,
		"the broker's utility pod":      brokerUtility,
		"the support services operator": supportStack,
		"a module's API server":         modulePod,
	} {
		assert.False(
			t, admits(t, policy, pod, cockroachSqlPort),
			"%s must have no path to the database", name,
		)
	}
}

// TestNothingButTheDatabaseIsRestricted covers the property that makes one
// policy the right amount.
//
// A NetworkPolicy restricts only the pods it selects, and leaves every other
// pod exactly as it was. Two earlier versions of this file fenced the whole
// namespace and broke things that were not visible from inside it - a
// module's controller, which is in no component list, and a module's API
// server, which is in another namespace entirely. Selecting the database
// alone cannot do that, and this is the check that keeps it so.
func TestNothingButTheDatabaseIsRestricted(t *testing.T) {
	policy := testInstaller("dev-0", "kubernetes-workload").databasePolicy()

	selector, found, err := unstructured.NestedMap(policy.Object, "spec", "podSelector")
	require.NoError(t, err)
	require.True(t, found)
	assert.NotEmpty(
		t, selector,
		"an empty pod selector would select every pod in the namespace and restrict all of them",
	)

	assert.True(t, selectorMatches(t, selector, databasePod), "the database is the pod being closed")
	for name, pod := range map[string]map[string]string{
		"the API server":                apiServerPod,
		"a controller":                  controllerPod,
		"the message broker":            brokerPod,
		"the broker's utility pod":      brokerUtility,
		"the support services operator": supportStack,
		"a maintenance job":             maintenanceJob,
		"a module's API server":         modulePod,
	} {
		assert.False(
			t, selectorMatches(t, selector, pod),
			"%s must not be selected; a policy that selects it starts restricting it", name,
		)
	}

	types, _, err := unstructured.NestedStringSlice(policy.Object, "spec", "policyTypes")
	require.NoError(t, err)
	assert.Equal(
		t, []string{"Ingress"}, types,
		"restricting the database's egress protects nothing this does not, and every direction named is one that can be wrong",
	)
}

// TestMaintenanceJobsCarryTheLabelTheDatabasePolicyAdmits covers the two
// halves of one decision.
//
// The policy admits pods carrying LabelMaintenance. A drop job that does not
// set it is denied the database, and the drop runs after the control plane
// has been scaled to zero - so the failure leaves the installation down
// rather than refusing to start.
func TestMaintenanceJobsCarryTheLabelTheDatabasePolicyAdmits(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "reinstall.go", nil, 0)
	require.NoError(t, err)

	jobs, labelled := 0, 0
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		keys := map[string]bool{}
		for _, element := range literal.Elts {
			keyed, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if basic, ok := keyed.Key.(*ast.BasicLit); ok {
				keys[basic.Value] = true
			}
		}
		if !keys[`"template"`] {
			return true
		}
		jobs++
		if containsIdent(literal, "LabelMaintenance") {
			labelled++
		}

		return true
	})

	require.NotZero(t, jobs, "no job template found to check")
	assert.Equal(
		t, jobs, labelled,
		"every maintenance job must carry the label the database policy admits",
	)
}

// TestReinstallReappliesTheNetworkPolicy covers the install path a
// behavioural test cannot reach.
//
// Reinstall deletes installer-managed resources and installs again. A
// database left with no policy is unrestricted rather than broken, which
// nothing would report.
func TestReinstallReappliesTheNetworkPolicy(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "reinstall.go", nil, 0)
	require.NoError(t, err)

	called := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			called = called || selector.Sel.Name == "InstallThreeportNetworkPolicies"
		}

		return true
	})

	assert.True(t, called, "reinstall must reapply the network policy")
}

// containsIdent reports whether the named identifier appears anywhere inside.
func containsIdent(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(inner ast.Node) bool {
		if ident, ok := inner.(*ast.Ident); ok && ident.Name == name {
			found = true
		}

		return !found
	})

	return found
}
