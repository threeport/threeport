package v0

import "testing"

// TestClusterBindingForKeepsALongerSibling covers a short namespace not owning
// a binding whose name starts with a longer installer-managed namespace.
func TestClusterBindingForKeepsALongerSibling(t *testing.T) {
	// compare ownership when foo and foo-bar both exist
	namespaces := []string{"foo", "foo-bar"}
	sibling := "foo-bar-rest-api-threeportworkloads"
	own := "foo-rest-api-threeportworkloads"
	if clusterBindingFor(sibling, "foo", namespaces) {
		t.Error("expected foo to leave the foo-bar binding")
	}
	if !clusterBindingFor(sibling, "foo-bar", namespaces) {
		t.Error("expected foo-bar to own its binding")
	}
	if !clusterBindingFor(own, "foo", namespaces) {
		t.Error("expected foo to own its own binding")
	}
	if clusterBindingFor("threeport-controllers-threeportworkloads", "foo", namespaces) {
		t.Error("expected the shared cluster role to be left")
	}
}

// TestClusterBindingForOwnsTheAgentBindings covers reinstall recreating an
// agent's own bindings and no one else's.
func TestClusterBindingForOwnsTheAgentBindings(t *testing.T) {
	namespaces := []string{"foo", "foo-bar", "foo-threeport"}
	for _, name := range []string{
		"foo-threeport-agent-manager-rolebinding",
		"foo-threeport-agent-proxy-rolebinding",
	} {
		if !clusterBindingFor(name, "foo", namespaces) {
			t.Errorf("expected foo to own %s", name)
		}
		// the sibling's own bindings do not match foo
		if clusterBindingFor("foo-bar-"+name[len("foo-"):], "foo", namespaces) {
			t.Errorf("expected foo to leave the foo-bar binding for %s", name)
		}
	}
	// another agent's bindings are not foo's, even when foo is a prefix
	if clusterBindingFor("foo-bar-threeport-agent-manager-rolebinding", "foo", namespaces) {
		t.Error("expected foo to leave the foo-bar agent binding")
	}
	// the pre-namespace names belong to no one
	if clusterBindingFor("threeport-agent-manager-rolebinding", "foo", namespaces) {
		t.Error("expected the old fixed-name binding to be left")
	}
}
