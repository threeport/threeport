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
