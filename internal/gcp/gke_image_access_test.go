package gcp

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	tpi "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// controlPlaneWithAgent returns a control plane instance that records an agent
// image from the namespace.
func controlPlaneWithAgent(imageNamespace string) *v0.ControlPlaneInstance {
	return &v0.ControlPlaneInstance{
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{Name: "rest-api", ImageNamespace: "other.example/threeport"},
			{Name: tpi.ThreeportAgentName, ImageNamespace: imageNamespace},
		},
	}
}

// TestNodeImageNamespaceFollowsTheProviderAccess covers which namespace a
// cluster's nodes are granted: the agent image's when the provider can manage
// its repository, none when it cannot, and none when no agent is recorded.
func TestNodeImageNamespaceFollowsTheProviderAccess(t *testing.T) {
	const namespace = "us-central1-docker.pkg.dev/image-project/threeport"

	for name, tc := range map[string]struct {
		controlPlane *v0.ControlPlaneInstance
		can          bool
		want         string
	}{
		"provider can manage":    {controlPlane: controlPlaneWithAgent(namespace), can: true, want: namespace},
		"provider cannot manage": {controlPlane: controlPlaneWithAgent(namespace), can: false, want: ""},
		"no agent recorded":      {controlPlane: &v0.ControlPlaneInstance{}, can: true, want: ""},
		"no control plane":       {controlPlane: nil, can: true, want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			// decide the namespace with a provider whose access is fixed
			got, err := nodeImageNamespace(
				tc.controlPlane,
				func(string) (bool, error) { return tc.can, nil },
			)

			// assert the namespace follows the agent record and the access
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestNodeImageNamespaceReportsAFailedAccessCheck covers a check that cannot be
// made: the error is returned so the caller can log it.
func TestNodeImageNamespaceReportsAFailedAccessCheck(t *testing.T) {
	// decide the namespace with a check that fails
	got, err := nodeImageNamespace(
		controlPlaneWithAgent("us-central1-docker.pkg.dev/image-project/threeport"),
		func(string) (bool, error) { return false, errors.New("no network") },
	)

	// assert the failure is returned with no namespace
	require.ErrorContains(t, err, "no network")
	assert.Empty(t, got)
}
