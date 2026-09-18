package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
)

// hostPortFor returns the host port a container port is published on, and
// whether the mappings name that container port more than once.
func hostPortFor(mappings []v1alpha4.PortMapping, containerPort int32) (int32, int) {
	var hostPort int32
	count := 0
	for _, mapping := range mappings {
		if mapping.ContainerPort == containerPort {
			hostPort = mapping.HostPort
			count++
		}
	}

	return hostPort, count
}

// TestGetPortMapping_DefaultsAreUnprivileged covers the ports a local install
// binds without being asked. Below 1024 works on a laptop only because dockerd
// runs as root and binds on the container's behalf, which is not available
// under rootless Docker, on hosts that raise ip_unprivileged_port_start, or on
// runners that reserve those ports.
func TestGetPortMapping_DefaultsAreUnprivileged(t *testing.T) {
	tests := []struct {
		name        string
		authEnabled bool
		wantPort    int32
	}{
		{name: "auth enabled", authEnabled: true, wantPort: 8443},
		{name: "auth disabled", authEnabled: false, wantPort: 8080},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mappings := getPortMapping(test.authEnabled, 0, nil)

			hostPort, count := hostPortFor(mappings, ThreeportAPINodePort)
			assert.Equal(t, 1, count)
			assert.Equal(t, test.wantPort, hostPort)
			assert.Greater(t, hostPort, int32(1023), "the default must not need a privileged bind")
		})
	}
}

// TestGetPortMapping_ApiPortOverridesTheDefault covers the flag, including the
// case it exists for: a user who wants the privileged port back.
func TestGetPortMapping_ApiPortOverridesTheDefault(t *testing.T) {
	mappings := getPortMapping(true, 443, nil)

	hostPort, count := hostPortFor(mappings, ThreeportAPINodePort)
	assert.Equal(t, 1, count)
	assert.Equal(t, int32(443), hostPort)
}

// TestGetPortMapping_UserMappingReplacesTheDefault is the bug the issue
// describes: a mapping for the API's container port used to be appended
// alongside the default rather than replacing it, so kind received two
// conflicting entries for one container port and --kind-port-mappings could not
// move the API at all.
func TestGetPortMapping_UserMappingReplacesTheDefault(t *testing.T) {
	mappings := getPortMapping(true, 0, map[int32]int32{ThreeportAPINodePort: 9443})

	hostPort, count := hostPortFor(mappings, ThreeportAPINodePort)
	require.Equal(t, 1, count, "one container port must be published once")
	assert.Equal(t, int32(9443), hostPort)
}

// TestGetPortMapping_KeepsOtherUserMappings covers the mappings the flag was
// there for in the first place.
func TestGetPortMapping_KeepsOtherUserMappings(t *testing.T) {
	mappings := getPortMapping(true, 0, map[int32]int32{
		ThreeportAPINodePort: 9443,
		30001:                4443,
		30002:                9180,
	})

	require.Len(t, mappings, 3)

	for containerPort, wantHostPort := range map[int32]int32{
		ThreeportAPINodePort: 9443,
		30001:                4443,
		30002:                9180,
	} {
		hostPort, count := hostPortFor(mappings, containerPort)
		assert.Equal(t, 1, count, "container port %d", containerPort)
		assert.Equal(t, wantHostPort, hostPort, "container port %d", containerPort)
	}
}
