// Package networkpolicytest checks what a Kubernetes network policy admits,
// the way a network policy is enforced, so a test can ask about the pods a
// policy is meant to let in and keep out instead of about its fields.
package networkpolicytest

import (
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// namespaceNameLabel is the label the Kubernetes API server sets on every
// namespace, always equal to its name.
const namespaceNameLabel = "kubernetes.io/metadata.name"

// The ports of the control plane's CockroachDB.
const (
	cockroachSqlPort     = 26257
	cockroachConsolePort = 8080
)

// databasePodLabels are the labels of the control plane's CockroachDB pods.
var databasePodLabels = map[string]string{"app.kubernetes.io/name": "cockroachdb"}

// Peer is a pod that opens a connection.
type Peer struct {
	// Namespace is the name of the pod's namespace.
	Namespace string

	// NamespaceLabels are the labels of the pod's namespace.  The label
	// kubernetes.io/metadata.name is always the namespace's name, whatever
	// these say, as the Kubernetes API server keeps it.
	NamespaceLabels map[string]string

	// PodLabels are the pod's labels.
	PodLabels map[string]string
}

// namespaceLabels returns the labels the peer's namespace carries.
func (p Peer) namespaceLabels() map[string]string {
	labels := map[string]string{}
	for key, value := range p.NamespaceLabels {
		labels[key] = value
	}
	labels[namespaceNameLabel] = p.Namespace

	return labels
}

// sliceField reads a list from obj without copying it, since a copy refuses
// the plain numbers an object built in Go can hold.
func sliceField(obj map[string]interface{}, fields ...string) ([]interface{}, bool, error) {
	value, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil || !found || value == nil {
		return nil, false, err
	}
	list, ok := value.([]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%v is a %T, not a list", fields, value)
	}

	return list, true, nil
}

// mapField reads an object from obj without copying it.
func mapField(obj map[string]interface{}, fields ...string) (map[string]interface{}, bool, error) {
	value, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil || !found || value == nil {
		return nil, false, err
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%v is a %T, not an object", fields, value)
	}

	return object, true, nil
}

// stringsField reads a list of strings from obj.
func stringsField(obj map[string]interface{}, fields ...string) ([]string, error) {
	list, _, err := sliceField(obj, fields...)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(list))
	for _, item := range list {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%v holds a %T, not a string", fields, item)
		}
		values = append(values, value)
	}

	return values, nil
}

// Selects reports whether the policy applies to pods carrying these labels in
// its own namespace.
func Selects(policy *unstructured.Unstructured, podLabels map[string]string) (bool, error) {
	selector, found, err := mapField(policy.Object, "spec", "podSelector")
	if err != nil {
		return false, fmt.Errorf("failed to read the policy's pod selector: %w", err)
	}
	if !found {
		return false, fmt.Errorf("the policy has no pod selector")
	}

	return selectorMatches(selector, podLabels)
}

// Admits reports whether one of the policy's ingress rules lets the peer
// connect on a TCP port.  A peer in a rule naming both a namespace selector
// and a pod selector has to match both; one naming only a pod selector admits
// pods of the policy's own namespace; one naming only a namespace selector
// admits every pod of the namespaces it matches.  It returns an error on a
// policy it cannot read, such as one with an IP block or a named port, rather
// than guess at it.
func Admits(policy *unstructured.Unstructured, peer Peer, port int) (bool, error) {
	rules, _, err := sliceField(policy.Object, "spec", "ingress")
	if err != nil {
		return false, fmt.Errorf("failed to read the policy's ingress rules: %w", err)
	}

	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			return false, fmt.Errorf("an ingress rule is not an object")
		}

		covered, err := ruleCoversPort(rule, port)
		if err != nil {
			return false, err
		}
		if !covered {
			continue
		}

		peers, found, err := sliceField(rule, "from")
		if err != nil {
			return false, fmt.Errorf("failed to read an ingress rule's sources: %w", err)
		}
		if !found || len(peers) == 0 {
			// a rule naming no sources admits every source
			return true, nil
		}

		for _, rawPeer := range peers {
			admitted, err := peerMatches(rawPeer, policy.GetNamespace(), peer)
			if err != nil {
				return false, err
			}
			if admitted {
				return true, nil
			}
		}
	}

	return false, nil
}

// peerMatches reports whether one source of an ingress rule matches the peer.
func peerMatches(rawPeer interface{}, policyNamespace string, peer Peer) (bool, error) {
	source, ok := rawPeer.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("an ingress rule's source is not an object")
	}
	if _, found := source["ipBlock"]; found {
		return false, fmt.Errorf("an ingress rule's source is an IP block, which this does not evaluate")
	}

	namespaceSelector, hasNamespaces, err := mapField(source, "namespaceSelector")
	if err != nil {
		return false, fmt.Errorf("failed to read a namespace selector: %w", err)
	}
	podSelector, hasPods, err := mapField(source, "podSelector")
	if err != nil {
		return false, fmt.Errorf("failed to read a pod selector: %w", err)
	}
	if !hasNamespaces && !hasPods {
		return false, fmt.Errorf("an ingress rule's source selects nothing")
	}

	if hasNamespaces {
		matches, err := selectorMatches(namespaceSelector, peer.namespaceLabels())
		if err != nil || !matches {
			return false, err
		}
	} else if peer.Namespace != policyNamespace {
		// a pod selector alone selects pods of the policy's namespace
		return false, nil
	}

	if hasPods {
		return selectorMatches(podSelector, peer.PodLabels)
	}

	return true, nil
}

// ruleCoversPort reports whether an ingress rule admits a TCP port.
func ruleCoversPort(rule map[string]interface{}, port int) (bool, error) {
	ports, found, err := sliceField(rule, "ports")
	if err != nil {
		return false, fmt.Errorf("failed to read an ingress rule's ports: %w", err)
	}
	if !found || len(ports) == 0 {
		// a rule naming no ports admits every port
		return true, nil
	}

	for _, rawPort := range ports {
		rulePort, ok := rawPort.(map[string]interface{})
		if !ok {
			return false, fmt.Errorf("an ingress rule's port is not an object")
		}
		if protocol, found := rulePort["protocol"]; found && protocol != "TCP" {
			continue
		}
		rawNumber, found := rulePort["port"]
		if !found {
			// a port entry naming only a protocol admits every port of it
			return true, nil
		}
		first, err := portNumber(rawNumber)
		if err != nil {
			return false, err
		}
		last := first
		if rawEnd, found := rulePort["endPort"]; found {
			if last, err = portNumber(rawEnd); err != nil {
				return false, err
			}
		}
		if port >= first && port <= last {
			return true, nil
		}
	}

	return false, nil
}

// portNumber reads a port number as an object built in Go or read from JSON
// holds it.
func portNumber(raw interface{}) (int, error) {
	switch number := raw.(type) {
	case int:
		return number, nil
	case int32:
		return int(number), nil
	case int64:
		return int(number), nil
	case float64:
		return int(number), nil
	default:
		return 0, fmt.Errorf("port %v is not a number, which this does not evaluate", raw)
	}
}

// selectorMatches reports whether a label selector matches labels.  An empty
// selector matches everything.
func selectorMatches(selector map[string]interface{}, labels map[string]string) (bool, error) {
	matchLabels, _, err := mapField(selector, "matchLabels")
	if err != nil {
		return false, fmt.Errorf("failed to read a selector's labels: %w", err)
	}
	for key, value := range matchLabels {
		if actual, found := labels[key]; !found || actual != value {
			return false, nil
		}
	}

	expressions, _, err := sliceField(selector, "matchExpressions")
	if err != nil {
		return false, fmt.Errorf("failed to read a selector's expressions: %w", err)
	}
	for _, rawExpression := range expressions {
		expression, ok := rawExpression.(map[string]interface{})
		if !ok {
			return false, fmt.Errorf("a selector's expression is not an object")
		}
		key, _, err := unstructured.NestedString(expression, "key")
		if err != nil {
			return false, fmt.Errorf("failed to read a selector expression's key: %w", err)
		}
		operator, _, err := unstructured.NestedString(expression, "operator")
		if err != nil {
			return false, fmt.Errorf("failed to read a selector expression's operator: %w", err)
		}
		values, err := stringsField(expression, "values")
		if err != nil {
			return false, fmt.Errorf("failed to read a selector expression's values: %w", err)
		}

		actual, found := labels[key]
		var matches bool
		switch operator {
		case "In":
			matches = found && slices.Contains(values, actual)
		case "NotIn":
			matches = !found || !slices.Contains(values, actual)
		case "Exists":
			matches = found
		case "DoesNotExist":
			matches = !found
		default:
			return false, fmt.Errorf("selector operator %q is not one a label selector has", operator)
		}
		if !matches {
			return false, nil
		}
	}

	return true, nil
}

// T is the part of *testing.T the assertions use.
type T interface {
	Helper()
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// AssertModuleDatabasePolicy checks a module's database network policy: it
// lives in the control plane's namespace, restricts only ingress to the
// control plane's database, and admits the module's API server pods from the
// module's namespace on the SQL port and nothing else.  In particular it
// keeps out the module's other pods, an unlabelled pod in the module's
// namespace, a pod of another namespace carrying the API server's labels and
// labelled to look like a module's, the control plane's own controllers and
// message broker, and another module's API server.  The policy is owned by
// the module's namespace, so it goes when the namespace does.
func AssertModuleDatabasePolicy(
	t T,
	policy *unstructured.Unstructured,
	controlPlaneNamespace string,
	moduleNamespace string,
	apiServerPodLabels map[string]string,
) {
	t.Helper()

	if policy.GetKind() != "NetworkPolicy" {
		t.Fatalf("expected a NetworkPolicy, got a %q", policy.GetKind())
	}
	if policy.GetNamespace() != controlPlaneNamespace {
		t.Errorf(
			"expected the policy in the control plane's namespace %q, beside the database, got %q",
			controlPlaneNamespace, policy.GetNamespace(),
		)
	}

	policyTypes, err := stringsField(policy.Object, "spec", "policyTypes")
	if err != nil {
		t.Fatalf("failed to read the policy types: %v", err)
	}
	if !slices.Equal(policyTypes, []string{"Ingress"}) {
		t.Errorf("expected the policy to restrict ingress only, got policy types %v", policyTypes)
	}

	selector, _, err := mapField(policy.Object, "spec", "podSelector")
	if err != nil {
		t.Fatalf("failed to read the pod selector: %v", err)
	}
	if len(selector) == 0 {
		t.Errorf("expected the policy to select the database's pods, but its empty pod selector selects every pod")
	}
	assertSelects(t, policy, databasePodLabels, true, "the control plane's database")
	assertSelects(t, policy, map[string]string{"app.kubernetes.io/name": "threeport-api-server"}, false, "the control plane's API server")

	if len(apiServerPodLabels) == 0 {
		t.Fatalf("expected the module's API server pod labels, got none")
	}
	assertAdmits(t, policy, Peer{Namespace: moduleNamespace, PodLabels: apiServerPodLabels}, cockroachSqlPort, true,
		"the module's API server, whose db-init and database-migrator reach its database")
	assertAdmits(t, policy, Peer{Namespace: moduleNamespace, PodLabels: apiServerPodLabels}, cockroachConsolePort, false,
		"the module's API server, on the database console's port")

	// probing one other port would pass a policy opening a third, so every
	// port each rule opens has to be the SQL port
	for _, problem := range portsOtherThan(policy, cockroachSqlPort) {
		t.Errorf("expected the policy to open the SQL port %d only, but %s", cockroachSqlPort, problem)
	}

	imitation := map[string]string{"threeport.io/module": "true"}
	for key, value := range apiServerPodLabels {
		imitation[key] = value
	}
	blocked := map[string]Peer{
		"an unlabelled pod in the module's namespace": {Namespace: moduleNamespace},
		"another pod of the module's namespace": {
			Namespace: moduleNamespace,
			PodLabels: map[string]string{"app.kubernetes.io/name": moduleNamespace + "-controller"},
		},
		"a pod carrying the module API server's labels in another namespace labelled like a module's": {
			Namespace:       "not-" + moduleNamespace,
			NamespaceLabels: imitation,
			PodLabels:       apiServerPodLabels,
		},
		"a control plane controller": {
			Namespace: controlPlaneNamespace,
			PodLabels: map[string]string{"app.kubernetes.io/name": "threeport-kubernetes-workload-controller"},
		},
		"the control plane's message broker": {
			Namespace: controlPlaneNamespace,
			PodLabels: map[string]string{"app.kubernetes.io/name": "nats"},
		},
		"another module's API server": {
			Namespace: "threeport-other-module",
			PodLabels: map[string]string{"app.kubernetes.io/name": "threeport-other-module-api-server"},
		},
	}
	for name, peer := range blocked {
		assertAdmits(t, policy, peer, cockroachSqlPort, false, name)
	}

	owned := false
	for _, owner := range policy.GetOwnerReferences() {
		if owner.APIVersion == "v1" && owner.Kind == "Namespace" && owner.Name == moduleNamespace {
			owned = true
		}
	}
	if !owned {
		t.Errorf("expected the policy to be owned by namespace %q, so it goes when the module's namespace does", moduleNamespace)
	}
}

// portsOtherThan describes each way the policy's ingress rules open more
// than one TCP port.
func portsOtherThan(policy *unstructured.Unstructured, port int) []string {
	rules, _, err := sliceField(policy.Object, "spec", "ingress")
	if err != nil {
		return []string{err.Error()}
	}

	var problems []string
	for index, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("ingress rule %d is not an object", index))
			continue
		}
		ports, _, err := sliceField(rule, "ports")
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if len(ports) == 0 {
			problems = append(problems, fmt.Sprintf("ingress rule %d names no ports, so it opens every one", index))
			continue
		}
		for _, rawPort := range ports {
			rulePort, ok := rawPort.(map[string]interface{})
			if !ok {
				problems = append(problems, fmt.Sprintf("ingress rule %d has a port that is not an object", index))
				continue
			}
			if protocol, found := rulePort["protocol"]; found && protocol != "TCP" {
				problems = append(problems, fmt.Sprintf("ingress rule %d opens protocol %v", index, protocol))
				continue
			}
			rawNumber, found := rulePort["port"]
			if !found {
				problems = append(problems, fmt.Sprintf("ingress rule %d opens every TCP port", index))
				continue
			}
			number, err := portNumber(rawNumber)
			if err != nil {
				problems = append(problems, fmt.Sprintf("ingress rule %d: %s", index, err))
				continue
			}
			last := number
			if rawEnd, found := rulePort["endPort"]; found {
				if last, err = portNumber(rawEnd); err != nil {
					problems = append(problems, fmt.Sprintf("ingress rule %d: %s", index, err))
					continue
				}
			}
			if number != port || last != port {
				problems = append(problems, fmt.Sprintf("ingress rule %d opens port %d to %d", index, number, last))
			}
		}
	}

	return problems
}

// assertSelects checks whether the policy applies to pods carrying labels.
func assertSelects(t T, policy *unstructured.Unstructured, podLabels map[string]string, want bool, name string) {
	t.Helper()

	selects, err := Selects(policy, podLabels)
	if err != nil {
		t.Fatalf("failed to evaluate the policy's pod selector: %v", err)
	}
	if selects != want {
		t.Errorf("expected the policy to select %s: %t, got %t", name, want, selects)
	}
}

// assertAdmits checks whether the policy admits a peer on a port.
func assertAdmits(t T, policy *unstructured.Unstructured, peer Peer, port int, want bool, name string) {
	t.Helper()

	admitted, err := Admits(policy, peer, port)
	if err != nil {
		t.Fatalf("failed to evaluate the policy: %v", err)
	}
	if admitted != want {
		t.Errorf("expected the policy to admit %s on port %d: %t, got %t", name, port, want, admitted)
	}
}
