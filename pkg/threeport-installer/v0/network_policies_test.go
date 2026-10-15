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

// The pods a control plane namespace holds, by the labels they carry. The
// last three are the ones a policy set written from the component list gets
// wrong: they arrive after the install, or carry labels of their own.
var (
	apiServerPod   = map[string]string{"app.kubernetes.io/name": ThreeportAPIServiceResourceName}
	databasePod    = map[string]string{"app.kubernetes.io/name": "cockroachdb"}
	brokerPod      = map[string]string{"app.kubernetes.io/name": "nats"}
	controllerPod  = map[string]string{"app.kubernetes.io/name": "threeport-kubernetes-workload-controller"}
	modulePod      = map[string]string{"app.kubernetes.io/name": "dev-0-spring-boot-controller"}
	brokerUtility  = map[string]string{"app": "nats-js-box"}
	supportStack   = map[string]string{"control-plane": "controller-manager"}
	maintenanceJob = map[string]string{LabelMaintenance: "true"}
)

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

// selectorMatches reports whether a pod selector matches a pod's labels. An
// empty selector matches every pod in the namespace, which is what the
// namespace-scoped rules rely on.
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

// ruleReaches reports whether one ingress or egress rule covers a peer on a
// port. A rule with no peer list covers every peer, which is how the rules for
// a Kubernetes API endpoint are written.
func ruleReaches(
	t *testing.T,
	rule map[string]interface{},
	peerKey string,
	peer map[string]string,
	port int,
) bool {
	t.Helper()

	ports, found, err := unstructured.NestedSlice(rule, "ports")
	require.NoError(t, err)
	if found {
		matched := false
		for _, raw := range ports {
			number, _, err := unstructured.NestedInt64(raw.(map[string]interface{}), "port")
			require.NoError(t, err)
			if number == int64(port) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	peers, found, err := unstructured.NestedSlice(rule, peerKey)
	require.NoError(t, err)
	if !found {
		// no peer list at all: every destination, which is only used for
		// ports that identify what is being reached
		return true
	}
	for _, raw := range peers {
		entry := raw.(map[string]interface{})
		// a peer in another namespace cannot be one of this namespace's pods
		if _, isRemote, _ := unstructured.NestedMap(entry, "namespaceSelector"); isRemote {
			continue
		}
		selector, found, err := unstructured.NestedMap(entry, "podSelector")
		require.NoError(t, err)
		if found && selectorMatches(t, selector, peer) {
			return true
		}
	}

	return false
}

// allows reports whether any policy selecting the pod permits the rule.
func allows(
	t *testing.T,
	policies []*unstructured.Unstructured,
	pod map[string]string,
	direction string,
	peerKey string,
	peer map[string]string,
	port int,
) bool {
	t.Helper()

	for _, policy := range policies {
		selector, _, err := unstructured.NestedMap(policy.Object, "spec", "podSelector")
		require.NoError(t, err)
		if !selectorMatches(t, selector, pod) {
			continue
		}
		rules, found, err := unstructured.NestedSlice(policy.Object, "spec", direction)
		require.NoError(t, err)
		if !found {
			continue
		}
		for _, raw := range rules {
			if ruleReaches(t, raw.(map[string]interface{}), peerKey, peer, port) {
				return true
			}
		}
	}

	return false
}

// canReach answers the question a policy set is actually for: may this pod
// open this connection?
//
// It checks both ends, because a NetworkPolicy only ever adds permission and
// the two directions are separate decisions. Allowing the egress and
// forgetting the ingress reads as a working rule and denies the connection -
// which is how the broker's utility pod was granted a path it never had.
func canReach(
	t *testing.T,
	policies []*unstructured.Unstructured,
	from, to map[string]string,
	port int,
) bool {
	t.Helper()

	return allows(t, policies, from, "egress", "to", to, port) &&
		allows(t, policies, to, "ingress", "from", from, port)
}

// TestDefaultDenyIsPresent covers the policy the others depend on.
//
// A NetworkPolicy only adds permission, so without one selecting every pod in
// both directions the allow rules describe paths that were already open and
// the whole set means nothing.
func TestDefaultDenyIsPresent(t *testing.T) {
	policies := testInstaller("dev-0", "kubernetes-workload").threeportNetworkPolicies()
	deny := policyNamed(t, policies, "threeport-default-deny")

	selector, _, err := unstructured.NestedMap(deny.Object, "spec", "podSelector")
	require.NoError(t, err)
	assert.Empty(t, selector, "the default deny has to select every pod")

	types, _, err := unstructured.NestedStringSlice(deny.Object, "spec", "policyTypes")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Ingress", "Egress"}, types)

	for _, direction := range []string{"ingress", "egress"} {
		_, found, err := unstructured.NestedSlice(deny.Object, "spec", direction)
		require.NoError(t, err)
		assert.False(t, found, "the default deny must carry no %s rule", direction)
	}
}

// TestEveryPodInTheNamespaceReachesWhatItNeeds covers the reason these rules
// select by namespace rather than by component.
//
// The namespace's population changes after the install: a module brings its
// own controller, a reinstall can enable a different set, and maintenance jobs
// appear and finish. A rule naming the components present at install denies
// every later arrival, and denies it silently - the pod starts, reports ready,
// and cannot talk to anything. The pods below include three that no component
// list contains.
func TestEveryPodInTheNamespaceReachesWhatItNeeds(t *testing.T) {
	policies := testInstaller("dev-0", "kubernetes-workload").threeportNetworkPolicies()

	for name, pod := range map[string]map[string]string{
		"a controller this install deployed": controllerPod,
		"a controller a module brought":      modulePod,
		"the broker's utility pod":           brokerUtility,
		"the support services operator":      supportStack,
		"a maintenance job":                  maintenanceJob,
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(
				t, canReach(t, policies, pod, apiServerPod, threeportApiPort),
				"%s must reach the API server", name,
			)
			assert.True(
				t, canReach(t, policies, pod, brokerPod, natsClientPort),
				"%s must reach the message broker", name,
			)
		})
	}
}

// TestOnlyTheApiServerReachesTheDatabase covers the restriction that carries
// the value.
//
// The controllers hold cluster-admin on the cluster they run in. None of them
// talks to the database - they go through the API server - so a foothold in
// one has no path to the credentials and state the database holds. This is the
// one place the namespace is not treated as a unit, and it is the reason the
// rest of it can be.
func TestOnlyTheApiServerReachesTheDatabase(t *testing.T) {
	policies := testInstaller("dev-0", "kubernetes-workload").threeportNetworkPolicies()

	assert.True(
		t, canReach(t, policies, apiServerPod, databasePod, cockroachSqlPort),
		"the API server is the component that reads the database",
	)
	assert.True(
		t, canReach(t, policies, maintenanceJob, databasePod, cockroachSqlPort),
		"a drop runs while the API server is scaled to zero, so it reaches the database itself",
	)

	for name, pod := range map[string]map[string]string{
		"a controller this install deployed": controllerPod,
		"a controller a module brought":      modulePod,
		"the broker's utility pod":           brokerUtility,
		"the support services operator":      supportStack,
	} {
		assert.False(
			t, canReach(t, policies, pod, databasePod, cockroachSqlPort),
			"%s must have no path to the database", name,
		)
	}
}

// TestTheDatabaseIsClosedFromBothEnds covers the claim separately from the
// reachability it produces.
//
// The database is unreachable because the egress is withheld and the ingress
// is restricted. Either alone makes it unreachable, which is the point -
// but it also means a test that only asks whether a pod can reach the
// database keeps passing when one of the two halves is opened up, and the
// restriction then rests on a single rule without anything saying so.
func TestTheDatabaseIsClosedFromBothEnds(t *testing.T) {
	policies := testInstaller("dev-0", "kubernetes-workload").threeportNetworkPolicies()

	for name, pod := range map[string]map[string]string{
		"a controller this install deployed": controllerPod,
		"a controller a module brought":      modulePod,
		"the broker's utility pod":           brokerUtility,
		"the support services operator":      supportStack,
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(
				t, allows(t, policies, pod, "egress", "to", databasePod, cockroachSqlPort),
				"%s must not be granted egress to the database", name,
			)
			assert.False(
				t, allows(t, policies, databasePod, "ingress", "from", pod, cockroachSqlPort),
				"the database must not admit %s", name,
			)
		})
	}
}

// TestARemoteKubernetesApiIsReachable covers the destination that has no
// selector.
//
// The controllers deploy workloads to runtimes other than their own, and a
// remote cluster's endpoint is a provider address rather than anything a
// selector describes - so the port is all the rule has to go on. GKE and EKS
// publish 443; OKE publishes 6443, and leaving it out denies every workload
// deployment and child control plane on that provider.
func TestARemoteKubernetesApiIsReachable(t *testing.T) {
	policies := testInstaller("dev-0", "kubernetes-workload").threeportNetworkPolicies()
	somewhereOutside := map[string]string{"app.kubernetes.io/name": "a-remote-kubernetes-api"}

	for _, port := range []int{kubernetesApiPort, kubernetesApiAltPort} {
		assert.True(
			t, allows(t, policies, controllerPod, "egress", "to", somewhereOutside, port),
			"a controller must reach a Kubernetes API on %d", port,
		)
	}
}

// TestMaintenanceJobsCarryTheLabelTheDatabasePolicyAdmits covers the two
// halves of one decision.
//
// databasePolicy admits pods carrying LabelMaintenance. A drop job that does
// not set it is denied the database, and the drop runs after the control plane
// has been scaled to zero - so the failure leaves the installation down rather
// than refusing to start.
func TestMaintenanceJobsCarryTheLabelTheDatabasePolicyAdmits(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "reinstall.go", nil, 0)
	require.NoError(t, err)

	jobs, labelled := 0, 0
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		// the pod template of a Job: a map literal with a "spec" holding a
		// restartPolicy, which only a pod spec carries
		source := literalKeys(literal)
		if !slices.Contains(source, "\"template\"") {
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

// TestReinstallReappliesTheNetworkPolicies covers the install path a
// behavioural test cannot reach.
//
// Reinstall deletes installer-managed resources and installs again, and it can
// enable a different set of controllers than the one running before. The
// policies describe the namespace rather than a component list, so they do not
// go stale - but they do get deleted, and a namespace left with no default
// deny is unrestricted rather than broken, which nothing would report.
func TestReinstallReappliesTheNetworkPolicies(t *testing.T) {
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

	assert.True(t, called, "reinstall must reapply the network policies")
}

// literalKeys returns the source text of a composite literal's keys.
func literalKeys(literal *ast.CompositeLit) []string {
	var keys []string
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if basic, ok := keyed.Key.(*ast.BasicLit); ok {
			keys = append(keys, basic.Value)
		}
	}

	return keys
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
