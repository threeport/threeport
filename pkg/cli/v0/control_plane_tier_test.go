package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestKindCannotRunAProductionTier covers the one provider/tier pairing that
// cannot be honoured.
//
// kind is a local, single-node, ephemeral cluster. The tier gates replica
// counts, database topology and backups, so accepting production there would
// report a production control plane and install a development one - and it has
// to be rejected before any infrastructure is touched, since by then the
// operator has a cluster they were told is something it is not.
func TestKindCannotRunAProductionTier(t *testing.T) {
	// api port is only accepted for kind, so it is passed only there
	validate := func(provider, tier string) error {
		apiPort := 0
		if provider == v0.KubernetesRuntimeInfraProviderKind {
			apiPort = 443
		}

		return ValidateCreateGenesisControlPlaneFlags(
			"test", provider, tier, "", false, nil, false, "", apiPort,
		)
	}

	err := validate(v0.KubernetesRuntimeInfraProviderKind, threeport.ControlPlaneTierProd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), threeport.ControlPlaneTierProd)
	assert.Contains(t, err.Error(), v0.KubernetesRuntimeInfraProviderKind)

	assert.NoError(
		t, validate(v0.KubernetesRuntimeInfraProviderKind, threeport.ControlPlaneTierDev),
		"kind runs the development tier",
	)
	assert.NoError(
		t, validate(v0.KubernetesRuntimeInfraProviderEKS, threeport.ControlPlaneTierProd),
		"every other provider may run the production tier",
	)
}

// TestDefaultTierForProviderIsRecordedNotInferred covers the reason the field
// exists: the default is a function of the provider at install time, and
// nothing afterwards can work it out again.
//
// A child control plane is created by the control-plane-controller from a
// stored definition. If the tier is not on that definition, the controller has
// only the provider to go on, which is how a control plane deliberately
// installed as development on a cloud provider would silently come up as
// production.
func TestDefaultTierForProviderIsRecordedNotInferred(t *testing.T) {
	assert.Equal(
		t, threeport.ControlPlaneTierDev,
		threeport.DefaultControlPlaneTierForProvider(v0.KubernetesRuntimeInfraProviderKind),
	)
	assert.Equal(
		t, threeport.ControlPlaneTierProd,
		threeport.DefaultControlPlaneTierForProvider(v0.KubernetesRuntimeInfraProviderEKS),
	)

	// the field a definition carries it in
	definition := v0.ControlPlaneDefinition{}
	assert.Nil(t, definition.Tier, "a definition with no tier recorded reports none, rather than guessing one")
}

// TestEveryControlPlaneDefinitionRecordsItsTier covers the half of this that a
// behavioural test cannot reach: the definition is built inside the genesis
// install, which needs a cluster to run.
//
// A definition created without a tier takes the column default, so the field
// is never empty and nothing downstream can tell a recorded development tier
// from one that was simply not written. That is the failure this field exists
// to remove, so the check is that every construction site sets it.
func TestEveryControlPlaneDefinitionRecordsItsTier(t *testing.T) {
	fileSet := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	constructed := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "ControlPlaneDefinition" {
				return true
			}
			constructed++

			setsTier := false
			for _, element := range literal.Elts {
				keyed, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := keyed.Key.(*ast.Ident); ok && key.Name == "Tier" {
					setsTier = true
				}
			}
			assert.True(
				t, setsTier,
				"%s: a control plane definition is created without recording its tier",
				fileSet.Position(literal.Pos()),
			)

			return true
		})
	}

	require.NotZero(t, constructed, "no control plane definition construction found to check")
}
