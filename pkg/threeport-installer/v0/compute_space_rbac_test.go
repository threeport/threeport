package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComputeSpaceWorkloadControllerSubject covers which namespace ends up in the
// principal a managed cluster is told to authorize.
//
// Two namespaces are in play and only one belongs here: the one the control
// plane's controllers run in on their host, not the one this installer creates
// on the managed cluster. A genesis install can be given any namespace, and
// naming the installer's instead authorizes a principal that never connects.
func TestComputeSpaceWorkloadControllerSubject(t *testing.T) {
	assert.Equal(
		t,
		"serviceAccount:a-project.svc.id.goog[a-custom-namespace/helm-workload-controller]",
		computeSpaceWorkloadControllerSubject("a-project", "a-custom-namespace", "helm-workload-controller"),
	)
}

// the installer's own namespace is where components go on the managed cluster,
// and must not stand in for the control plane's
func TestComputeSpaceWorkloadControllerSubject_IgnoresTheInstallerNamespace(t *testing.T) {
	cpi := NewInstaller()

	subject := computeSpaceWorkloadControllerSubject("a-project", "a-custom-namespace", "helm-workload-controller")

	assert.NotContains(t, subject, cpi.Opts.Namespace)
	assert.Contains(t, subject, "a-custom-namespace")
}
