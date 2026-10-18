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
)

// TestValidateTierForProvider covers the one provider and tier that cannot go
// together.
//
// kind is a local, single-node, ephemeral cluster. The tier gates replica
// counts, database topology and backups, so accepting production there would
// report a production control plane and install a development one.
func TestValidateTierForProvider(t *testing.T) {
	err := ValidateTierForProvider(v0.KubernetesRuntimeInfraProviderKind, ControlPlaneTierProd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), v0.KubernetesRuntimeInfraProviderKind)
	assert.Contains(t, err.Error(), ControlPlaneTierProd)

	assert.NoError(
		t, ValidateTierForProvider(v0.KubernetesRuntimeInfraProviderKind, ControlPlaneTierDev),
		"kind runs the development tier",
	)
	for _, provider := range []string{
		v0.KubernetesRuntimeInfraProviderEKS,
		v0.KubernetesRuntimeInfraProviderGKE,
	} {
		assert.NoError(
			t, ValidateTierForProvider(provider, ControlPlaneTierProd),
			"%s may run the production tier", provider,
		)
	}
}

// TestBothInstallPathsCheckTheProviderTierPairing covers the half of this a
// behavioural test cannot reach.
//
// A control plane is installed two ways. The genesis path is reachable from a
// test, and TestKindCannotRunAProductionTier in pkg/cli covers it. The child
// path runs inside the control-plane-controller against a real runtime and a
// real cluster, so the check it performs can only be read rather than run -
// and an install path that stopped checking would not fail anywhere, it would
// build a kind control plane and report it as production, which is the exact
// outcome the check exists to prevent.
func TestBothInstallPathsCheckTheProviderTierPairing(t *testing.T) {
	// repo root, from pkg/threeport-installer/v0
	root := filepath.Join("..", "..", "..")
	paths := map[string]string{
		"the genesis install": filepath.Join(root, "pkg", "cli", "v0", "genesis_control_plane.go"),
		"the child install":   filepath.Join(root, "internal", "control-plane", "v0_control_plane_instance.go"),
	}

	for description, path := range paths {
		t.Run(description, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoError(t, err)

			called := false
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch function := call.Fun.(type) {
				case *ast.SelectorExpr:
					called = called || function.Sel.Name == "ValidateTierForProvider"
				case *ast.Ident:
					called = called || function.Name == "ValidateTierForProvider"
				}

				return true
			})

			assert.True(
				t, called,
				"%s does not check the provider against the tier; %s would install a development control plane and record it as production",
				path, v0.KubernetesRuntimeInfraProviderKind,
			)
		})
	}
}
