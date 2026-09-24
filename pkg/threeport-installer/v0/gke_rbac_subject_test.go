package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The subject in a managed cluster's binding has to be the identity the control
// plane actually arrives as. Binding the wrong one authorizes a principal that
// does not exist and leaves the controllers with no access at all, which is the
// same outcome as installing no binding.
func TestComputeSpaceWorkloadControllerSubject(t *testing.T) {
	const (
		project    = "a-project"
		namespace  = "threeport-control-plane"
		controller = "helm-workload-controller"
		email      = "threeport@a-project.iam.gserviceaccount.com"
	)

	// a GKE-hosted control plane presents each controller's own Workload
	// Identity principal
	t.Run("workload identity principal when there is no service account", func(t *testing.T) {
		assert.Equal(
			t,
			"serviceAccount:a-project.svc.id.goog[threeport-control-plane/helm-workload-controller]",
			computeSpaceWorkloadControllerSubject(project, namespace, controller, ""),
		)
	})

	// hosted anywhere else, every controller arrives as the provider's service
	// account, whatever the managed cluster's project is
	t.Run("the service account when one is given", func(t *testing.T) {
		assert.Equal(t, email, computeSpaceWorkloadControllerSubject(project, namespace, controller, email))
		assert.Equal(t, email, computeSpaceWorkloadControllerSubject("", namespace, controller, email))
	})
}
