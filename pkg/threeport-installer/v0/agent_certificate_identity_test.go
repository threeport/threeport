package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// TestRuntimeControllerCertificateNameMatchesTheAPICheck covers the name the
// API accepts agent certificate requests from.  The installer issues each
// controller a certificate whose common name is the controller's name, so if
// the two ever differ the kubernetes runtime controller is refused its agents'
// certificates and every managed cluster's agent stops getting one.
func TestRuntimeControllerCertificateNameMatchesTheAPICheck(t *testing.T) {
	assert.Equal(t, ThreeportKubernetesRuntimeControllerName, auth.CNKubernetesRuntimeController)
}
