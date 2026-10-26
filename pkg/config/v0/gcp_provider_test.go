package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	provider "github.com/threeport/threeport/internal/provider"
)

// TestGcpProviderConfig_Validate_NameLength covers the usability cap on a
// provider name: a name a service account ID once could not hold is accepted,
// and an unreasonable one is not.
func TestGcpProviderConfig_Validate_NameLength(t *testing.T) {
	config := func(name string) *GcpProviderConfig {
		project, region := "a-project", "us-central1"
		return &GcpProviderConfig{GcpProvider: GcpProviderValues{
			Name: &name, ProjectID: &project, DefaultRegion: &region,
		}}
	}

	require.NoError(t, config("my-long-provider-name").Validate())
	require.NoError(t, config(strings.Repeat("a", provider.GcpProviderNameMaxLength)).Validate())

	err := config(strings.Repeat("a", provider.GcpProviderNameMaxLength+1)).Validate()
	require.ErrorContains(t, err, "too long")
}
