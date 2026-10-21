package networkpolicytest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// testPolicy returns a policy in namespace control-plane that selects the
// database and has the given ingress rules.
func testPolicy(rules ...interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata":   map[string]interface{}{"name": "test", "namespace": "control-plane"},
		"spec": map[string]interface{}{
			"podSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app.kubernetes.io/name": "cockroachdb"},
			},
			"policyTypes": []interface{}{"Ingress"},
			"ingress":     rules,
		},
	}}
}

// rule returns an ingress rule from sources on one TCP port.
func rule(port interface{}, sources ...interface{}) map[string]interface{} {
	return map[string]interface{}{
		"from":  sources,
		"ports": []interface{}{map[string]interface{}{"protocol": "TCP", "port": port}},
	}
}

func matchLabels(key, value string) map[string]interface{} {
	return map[string]interface{}{"matchLabels": map[string]interface{}{key: value}}
}

var (
	apiPod   = map[string]string{"app": "api"}
	otherPod = map[string]string{"app": "other"}
)

func admits(t *testing.T, policy *unstructured.Unstructured, peer Peer, port int) bool {
	t.Helper()
	admitted, err := Admits(policy, peer, port)
	require.NoError(t, err)

	return admitted
}

func TestAPeerNamingANamespaceAndPodsNeedsBoth(t *testing.T) {
	policy := testPolicy(rule(int64(26257), map[string]interface{}{
		"namespaceSelector": matchLabels(namespaceNameLabel, "module"),
		"podSelector":       matchLabels("app", "api"),
	}))

	assert.True(t, admits(t, policy, Peer{Namespace: "module", PodLabels: apiPod}, 26257))
	assert.False(t, admits(t, policy, Peer{Namespace: "module", PodLabels: otherPod}, 26257), "the pods have to match")
	assert.False(t, admits(t, policy, Peer{Namespace: "elsewhere", PodLabels: apiPod}, 26257), "the namespace has to match")
	assert.False(t, admits(t, policy, Peer{Namespace: "module", PodLabels: apiPod}, 8080), "the port has to match")
}

func TestTwoPeersAdmitEitherOne(t *testing.T) {
	policy := testPolicy(rule(int64(26257),
		map[string]interface{}{"namespaceSelector": matchLabels(namespaceNameLabel, "module")},
		map[string]interface{}{"podSelector": matchLabels("app", "api")},
	))

	assert.True(t, admits(t, policy, Peer{Namespace: "module", PodLabels: otherPod}, 26257), "a namespace selector alone admits every pod of it")
	assert.True(t, admits(t, policy, Peer{Namespace: "control-plane", PodLabels: apiPod}, 26257), "a pod selector alone admits pods of the policy's namespace")
	assert.False(t, admits(t, policy, Peer{Namespace: "elsewhere", PodLabels: apiPod}, 26257), "a pod selector alone admits no other namespace")
}

func TestANamespaceIsKnownByItsName(t *testing.T) {
	policy := testPolicy(rule(int64(26257), map[string]interface{}{
		"namespaceSelector": matchLabels(namespaceNameLabel, "module"),
	}))

	assert.False(
		t,
		admits(t, policy, Peer{Namespace: "elsewhere", NamespaceLabels: map[string]string{namespaceNameLabel: "module"}}, 26257),
		"the Kubernetes API server keeps the name label equal to the namespace's name",
	)
}

func TestRulesAdmitWhatTheyLeaveOpen(t *testing.T) {
	anyone := Peer{Namespace: "elsewhere", PodLabels: otherPod}

	assert.True(t, admits(t, testPolicy(map[string]interface{}{
		"ports": []interface{}{map[string]interface{}{"port": int64(26257)}},
	}), anyone, 26257), "a rule naming no sources admits every source")
	assert.True(t, admits(t, testPolicy(map[string]interface{}{
		"from": []interface{}{map[string]interface{}{"namespaceSelector": map[string]interface{}{}}},
	}), anyone, 1234), "a rule naming no ports admits every port, and an empty selector matches everything")
	assert.False(t, admits(t, testPolicy(), anyone, 26257), "a policy with no rules admits nothing")
	assert.False(t, admits(t, testPolicy(map[string]interface{}{
		"ports": []interface{}{map[string]interface{}{"protocol": "UDP", "port": int64(26257)}},
	}), anyone, 26257), "a UDP port is not a TCP port")
}

func TestPortsAreReadAsTheyAreStored(t *testing.T) {
	peer := Peer{Namespace: "control-plane", PodLabels: apiPod}
	for _, port := range []interface{}{int64(26257), int(26257), int32(26257), float64(26257)} {
		policy := testPolicy(rule(port, map[string]interface{}{"podSelector": matchLabels("app", "api")}))
		assert.True(t, admits(t, policy, peer, 26257), "port as %T", port)
	}

	ranged := testPolicy(map[string]interface{}{
		"ports": []interface{}{map[string]interface{}{"port": int64(26000), "endPort": int64(26300)}},
	})
	assert.True(t, admits(t, ranged, peer, 26257))
	assert.False(t, admits(t, ranged, peer, 26301))
}

func TestSelectorExpressions(t *testing.T) {
	expression := func(operator string, values ...interface{}) *unstructured.Unstructured {
		return testPolicy(rule(int64(26257), map[string]interface{}{
			"podSelector": map[string]interface{}{"matchExpressions": []interface{}{
				map[string]interface{}{"key": "app", "operator": operator, "values": values},
			}},
		}))
	}
	at := func(labels map[string]string) Peer { return Peer{Namespace: "control-plane", PodLabels: labels} }

	assert.True(t, admits(t, expression("In", "api"), at(apiPod), 26257))
	assert.False(t, admits(t, expression("In", "api"), at(otherPod), 26257))
	assert.False(t, admits(t, expression("NotIn", "api"), at(apiPod), 26257))
	assert.True(t, admits(t, expression("NotIn", "api"), at(nil), 26257))
	assert.True(t, admits(t, expression("Exists"), at(otherPod), 26257))
	assert.False(t, admits(t, expression("Exists"), at(nil), 26257))
	assert.True(t, admits(t, expression("DoesNotExist"), at(nil), 26257))
	assert.False(t, admits(t, expression("DoesNotExist"), at(apiPod), 26257))
}

func TestWhatItCannotEvaluateIsAnError(t *testing.T) {
	peer := Peer{Namespace: "control-plane", PodLabels: apiPod}
	for name, policy := range map[string]*unstructured.Unstructured{
		"an IP block": testPolicy(rule(int64(26257), map[string]interface{}{
			"ipBlock": map[string]interface{}{"cidr": "10.0.0.0/8"},
		})),
		"a named port": testPolicy(rule("sql", map[string]interface{}{"podSelector": matchLabels("app", "api")})),
		"an unknown operator": testPolicy(rule(int64(26257), map[string]interface{}{
			"podSelector": map[string]interface{}{"matchExpressions": []interface{}{
				map[string]interface{}{"key": "app", "operator": "Gt"},
			}},
		})),
		"a source selecting nothing": testPolicy(rule(int64(26257), map[string]interface{}{})),
	} {
		_, err := Admits(policy, peer, 26257)
		assert.Error(t, err, name)
	}
}

// recorder is a T that records failures, and stops at a fatal one the way
// *testing.T does.
type recorder struct {
	failures []string
}

type fatal struct{}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...interface{}) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...interface{}) {
	r.Errorf(format, args...)
	panic(fatal{})
}

// assertModuleDatabasePolicy returns the failures AssertModuleDatabasePolicy
// reports for a policy.
func assertModuleDatabasePolicy(policy *unstructured.Unstructured) (failures []string) {
	r := &recorder{}
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(fatal); !ok {
				panic(recovered)
			}
		}
		failures = r.failures
	}()
	AssertModuleDatabasePolicy(r, policy, "control-plane", "module", apiPod)

	return r.failures
}

// modulePolicy returns a module database policy as it should be.
func modulePolicy() *unstructured.Unstructured {
	policy := testPolicy(rule(int64(26257), map[string]interface{}{
		"namespaceSelector": matchLabels(namespaceNameLabel, "module"),
		"podSelector":       matchLabels("app", "api"),
	}))
	policy.Object["metadata"].(map[string]interface{})["ownerReferences"] = []interface{}{
		map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "module", "uid": "1"},
	}

	return policy
}

func TestAssertModuleDatabasePolicyPassesTheRightPolicy(t *testing.T) {
	assert.Empty(t, assertModuleDatabasePolicy(modulePolicy()))
}

func TestAssertModuleDatabasePolicyCatchesAWrongPolicy(t *testing.T) {
	for name, tc := range map[string]struct {
		change func(spec map[string]interface{}, metadata map[string]interface{})
		want   string
	}{
		"a module admitted by a label of threeport's own": {
			change: func(spec, _ map[string]interface{}) {
				spec["ingress"] = []interface{}{rule(int64(26257), map[string]interface{}{
					"namespaceSelector": matchLabels("threeport.io/module", "true"),
					"podSelector":       matchLabels("app", "api"),
				})}
			},
			want: "labelled like a module's",
		},
		"the namespace and pods in separate peers": {
			change: func(spec, _ map[string]interface{}) {
				spec["ingress"] = []interface{}{rule(int64(26257),
					map[string]interface{}{"namespaceSelector": matchLabels(namespaceNameLabel, "module")},
					map[string]interface{}{"podSelector": matchLabels("app", "api")},
				)}
			},
			want: "an unlabelled pod in the module's namespace",
		},
		"the whole namespace admitted": {
			change: func(spec, _ map[string]interface{}) {
				spec["ingress"] = []interface{}{rule(int64(26257), map[string]interface{}{
					"namespaceSelector": matchLabels(namespaceNameLabel, "module"),
				})}
			},
			want: "another pod of the module's namespace",
		},
		"the console's port admitted": {
			change: func(spec, _ map[string]interface{}) {
				rules := spec["ingress"].([]interface{})
				rules[0].(map[string]interface{})["ports"] = nil
			},
			want: "console",
		},
		"the API server's pods not admitted": {
			change: func(spec, _ map[string]interface{}) {
				spec["ingress"] = []interface{}{}
			},
			want: "whose db-init and database-migrator",
		},
		"every pod selected": {
			change: func(spec, _ map[string]interface{}) {
				spec["podSelector"] = map[string]interface{}{}
			},
			want: "empty pod selector",
		},
		"egress restricted too": {
			change: func(spec, _ map[string]interface{}) {
				spec["policyTypes"] = []interface{}{"Ingress", "Egress"}
			},
			want: "ingress only",
		},
		"in the module's namespace": {
			change: func(_, metadata map[string]interface{}) {
				metadata["namespace"] = "module"
			},
			want: "control plane's namespace",
		},
		"not owned by the module's namespace": {
			change: func(_, metadata map[string]interface{}) {
				delete(metadata, "ownerReferences")
			},
			want: "owned by namespace",
		},
	} {
		t.Run(name, func(t *testing.T) {
			policy := modulePolicy()
			tc.change(policy.Object["spec"].(map[string]interface{}), policy.Object["metadata"].(map[string]interface{}))

			failures := assertModuleDatabasePolicy(policy)
			require.NotEmpty(t, failures, "the assertion passed a policy with %s", name)
			assert.Contains(t, strings.Join(failures, "\n"), tc.want)
		})
	}
}
