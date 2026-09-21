package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetThreeportAPIPort covers the port a cloud install records. It is the
// load balancer's port, not a host port anything binds, so the unprivileged
// defaults a local install uses must not reach it: an EKS, GKE or OKE control
// plane that recorded 8443 would persist an endpoint no client can reach.
func TestGetThreeportAPIPort(t *testing.T) {
	assert.Equal(t, 443, GetThreeportAPIPort(true))
	assert.Equal(t, 80, GetThreeportAPIPort(false))
}

// TestGetLocalThreeportAPIPort covers the host port a local install publishes,
// which is the one the --api-port flag moves.
func TestGetLocalThreeportAPIPort(t *testing.T) {
	tests := []struct {
		name        string
		authEnabled bool
		apiPort     int
		want        int
	}{
		{name: "default with auth", authEnabled: true, want: DefaultLocalAPIPortAuthEnabled},
		{name: "default without auth", authEnabled: false, want: DefaultLocalAPIPortAuthDisabled},
		{name: "override", authEnabled: true, apiPort: 9443, want: 9443},
		{name: "override back to the privileged port", authEnabled: true, apiPort: 443, want: 443},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, GetLocalThreeportAPIPort(test.authEnabled, test.apiPort))
		})
	}

	assert.Greater(t, DefaultLocalAPIPortAuthEnabled, 1023, "the default must not need a privileged bind")
	assert.Greater(t, DefaultLocalAPIPortAuthDisabled, 1023, "the default must not need a privileged bind")
}
