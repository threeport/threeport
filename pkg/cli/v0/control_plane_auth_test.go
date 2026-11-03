package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
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

// TestTheChildInstallChecksTheProviderAgain covers the last moment before a
// child control plane exists.
//
// The API refuses the pairing when an instance is created and when its
// definition is updated, but the install reads the definition again, later
// than either. A check that stopped happening here would not fail: the child
// would come up serving its API over plain HTTP, which is a working control
// plane, just not the one that was asked for.
func TestTheChildInstallChecksTheProviderAgain(t *testing.T) {
	path := filepath.Join("..", "..", "..", "internal", "control-plane", "v0_control_plane_instance.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err)

	called := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			called = called || selector.Sel.Name == "ValidateAuthEnabledForProvider"
		}

		return true
	})

	assert.True(
		t, called,
		"%s installs a child without checking the auth setting against the provider", path,
	)
}
