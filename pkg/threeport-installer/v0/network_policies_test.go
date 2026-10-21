package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	networkpolicytest "github.com/threeport/threeport/pkg/networkpolicytest/v0"
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

// ruleCoversPort reports whether an ingress rule names a port.
func ruleCoversPort(t *testing.T, rule map[string]interface{}, port int) bool {
	t.Helper()

	ports, _, err := unstructured.NestedSlice(rule, "ports")
	require.NoError(t, err)
	for _, rawPort := range ports {
		number, _, err := unstructured.NestedInt64(rawPort.(map[string]interface{}), "port")
		require.NoError(t, err)
		if number == int64(port) {
			return true
		}
	}

	return false
}

// admits reports whether the policy lets a pod of this namespace open a
// connection on a port.
func admits(t *testing.T, policy *unstructured.Unstructured, pod map[string]string, port int) bool {
	t.Helper()

	rules, found, err := unstructured.NestedSlice(policy.Object, "spec", "ingress")
	require.NoError(t, err)
	require.True(t, found, "the policy carries no ingress rule, so it admits nothing")

	for _, rawRule := range rules {
		rule := rawRule.(map[string]interface{})
		if !ruleCoversPort(t, rule, port) {
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
	} {
		assert.False(
			t, admits(t, policy, pod, cockroachSqlPort),
			"%s must have no path to the database", name,
		)
	}

	assert.False(
		t, admits(t, policy, modulePod, cockroachSqlPort),
		"a module is admitted by its namespace, not by a pod of this one wearing its name",
	)
}

// TestTheDatabasePolicyAdmitsNoOtherNamespace covers what the module
// policies rely on.
//
// A module is admitted by a policy of its own, which matches its namespace
// by the name the Kubernetes API server keeps and its API server's pods by
// their labels. Policies selecting the same pods add up, so a rule here
// admitting a namespace by a label of threeport's own would let in any
// namespace labelled that way, whatever the module policies say - and
// anything allowed to patch a namespace can label one.
func TestTheDatabasePolicyAdmitsNoOtherNamespace(t *testing.T) {
	policy := testInstaller("dev-0", "kubernetes-workload").databasePolicy()

	for name, peer := range map[string]networkpolicytest.Peer{
		"a module's API server": {
			Namespace: "threeport-test",
			PodLabels: modulePod,
		},
		"a pod of a namespace labelled as a module's": {
			Namespace:       "someone-elses",
			NamespaceLabels: map[string]string{"threeport.io/module": "true"},
			PodLabels:       modulePod,
		},
		"a pod of another namespace carrying the API server's labels": {
			Namespace: "someone-elses",
			PodLabels: apiServerPod,
		},
	} {
		admitted, err := networkpolicytest.Admits(policy, peer, cockroachSqlPort)
		require.NoError(t, err)
		assert.False(t, admitted, "%s must be admitted by its own module policy, not this one", name)
	}
}

// TestTheSdkInstallsTheModuleDatabasePolicy covers the half of this that
// lives in another repository.
//
// The generated module installer creates the module's database policy, and
// nothing else admits a module to the database. They are written in
// different packages and nothing but this ties them together: a rename here
// compiles until a module is generated, and a generator that stopped calling
// it compiles too, and the module installs and then cannot reach its own
// data.
func TestTheSdkInstallsTheModuleDatabasePolicy(t *testing.T) {
	generator, err := parser.ParseFile(
		token.NewFileSet(),
		"../../sdk/v0/gen/pkg/installer/installer.go",
		nil,
		parser.ParseComments,
	)
	require.NoError(t, err)

	// the generator emits code rather than running it, so the reference is a
	// Qual("<this package>", "InstallModuleDatabasePolicy") naming the
	// function by string
	var installsPolicy bool
	ast.Inspect(generator, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		if calledFunc(call.Fun) != "Qual" {
			return true
		}

		pkg, pkgOk := stringLit(call.Args[0])
		name, nameOk := stringLit(call.Args[1])
		if pkgOk && nameOk &&
			strings.HasSuffix(pkg, "pkg/threeport-installer/v0") &&
			name == "InstallModuleDatabasePolicy" {
			installsPolicy = true
		}

		return true
	})

	assert.True(
		t, installsPolicy,
		"the SDK no longer installs the module's database policy, so every module generated "+
			"from it installs and then cannot reach its own database",
	)
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

// calledFunc returns the name of the function a call expression calls.
func calledFunc(fun ast.Expr) string {
	switch called := fun.(type) {
	case *ast.Ident:
		return called.Name
	case *ast.SelectorExpr:
		return called.Sel.Name
	}

	return ""
}

// stringLit returns the value of a string literal expression.
func stringLit(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)

	return value, err == nil
}
