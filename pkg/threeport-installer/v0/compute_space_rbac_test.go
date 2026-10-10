package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestComputeSpaceWorkloadControllerBinding_NamespaceSource covers which of two
// namespaces reaches the Workload Identity principal.
//
// Both are in play and only one belongs here: the namespace the control plane's
// controllers run in on their own host, not the one this installer creates
// components in on the managed cluster. A genesis install can be placed
// anywhere, so the two differ as soon as anyone uses a namespace other than the
// default - and naming the installer's authorizes a principal that never
// connects, leaving every operation against the cluster refused.
func TestComputeSpaceWorkloadControllerBinding_NamespaceSource(t *testing.T) {
	cpi := NewInstaller()
	cpi.Opts.Namespace = "where-components-go-on-the-managed-cluster"

	binding := cpi.computeSpaceWorkloadControllerBinding(
		"helm-workload-controller",
		"a-project",
		"",
		"where-the-control-plane-runs",
	)

	subjects, found, err := unstructured.NestedSlice(binding.Object, "subjects")
	assert.NoError(t, err)
	assert.True(t, found, "the binding must name a subject")

	name := subjects[0].(map[string]interface{})["name"].(string)
	assert.Equal(
		t,
		"serviceAccount:a-project.svc.id.goog[where-the-control-plane-runs/helm-workload-controller]",
		name,
	)
	assert.NotContains(
		t, name, cpi.Opts.Namespace,
		"the installer's own namespace must not stand in for the control plane's",
	)
}

// TestComputeSpaceWorkloadControllerBinding_ServiceAccountIgnoresNamespace
// covers the other identity. A control plane that arrives as a service account
// presents an email, which carries no namespace, so the control plane namespace
// has nothing to contribute and must not appear.
func TestComputeSpaceWorkloadControllerBinding_ServiceAccountIgnoresNamespace(t *testing.T) {
	cpi := NewInstaller()

	binding := cpi.computeSpaceWorkloadControllerBinding(
		"helm-workload-controller",
		"a-project",
		"threeport@a-project.iam.gserviceaccount.com",
		"where-the-control-plane-runs",
	)

	subjects, _, err := unstructured.NestedSlice(binding.Object, "subjects")
	assert.NoError(t, err)

	name := subjects[0].(map[string]interface{})["name"].(string)
	assert.Equal(t, "threeport@a-project.iam.gserviceaccount.com", name)
	assert.NotContains(t, name, "where-the-control-plane-runs")
}
