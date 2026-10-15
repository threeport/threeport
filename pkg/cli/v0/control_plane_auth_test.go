package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestAuthCannotBeDisabledOffKindFromTheCli covers the rail on the path an
// operator actually types.
//
// The flag's help text has claimed this restriction since it was written -
// "Can only be disabled when using kind provider" - and nothing enforced it.
// `tptctl up --provider eks --auth-enabled=false` provisioned a real cluster
// serving the API over plain HTTP, with a printed warning as the only thing
// between that and the network.
//
// It has to be refused before any infrastructure exists, because afterwards
// the operator has a cloud cluster that is already wrong.
func TestAuthCannotBeDisabledOffKindFromTheCli(t *testing.T) {
	// the api port is only accepted for kind, so it is passed only there
	validate := func(provider string, authEnabled bool) error {
		apiPort := 0
		if provider == v0.KubernetesRuntimeInfraProviderKind {
			apiPort = 443
		}

		return ValidateCreateGenesisControlPlaneFlags(
			"test", provider, threeport.ControlPlaneTierDev, "", authEnabled, nil, false, "", apiPort,
		)
	}

	for _, provider := range []string{
		v0.KubernetesRuntimeInfraProviderEKS,
		v0.KubernetesRuntimeInfraProviderGKE,
	} {
		err := validate(provider, false)
		require.Error(t, err, "%s must not be installed without auth", provider)
		assert.Contains(t, err.Error(), provider)

		assert.NoError(t, validate(provider, true), "%s with auth on is the normal case", provider)
	}

	assert.NoError(
		t, validate(v0.KubernetesRuntimeInfraProviderKind, false),
		"kind can still disable auth for local development",
	)
	assert.NoError(t, validate(v0.KubernetesRuntimeInfraProviderKind, true))
}
