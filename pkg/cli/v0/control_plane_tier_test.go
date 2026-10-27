package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
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
	// api port is only accepted for kind, so it is passed only there. auth is
	// left on throughout: this test is about the tier/provider pairing, and
	// ValidateCreateGenesisControlPlaneFlags also rejects auth disabled on
	// anything but kind - a separate check this test does not exercise.
	validate := func(provider, tier string) error {
		apiPort := 0
		if provider == v0.KubernetesRuntimeInfraProviderKind {
			apiPort = 443
		}

		return ValidateCreateGenesisControlPlaneFlags(
			"test", provider, tier, "", true, nil, false, "", apiPort,
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
// exists: the tier is chosen at install time, and nothing afterwards can work
// it out again.
//
// A child control plane is created by the control-plane-controller from a
// stored definition. If the tier is not on that definition, the controller has
// only the default to go on, which is how a control plane deliberately
// installed as production would silently come up as development.
func TestDefaultTierForProviderIsRecordedNotInferred(t *testing.T) {
	assert.Equal(
		t, threeport.ControlPlaneTierDev, threeport.DefaultControlPlaneTier,
		"production is opted into, never defaulted into, on any provider",
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
	var files []string
	// every package that builds a definition, not only this one: the combined
	// control-plane config builds one too, and shipped without a tier
	for _, dir := range []string{".", filepath.Join("..", "..", "config", "v0")} {
		found, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		files = append(files, found...)
	}

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
			// api_v0.ControlPlaneDefinition{...} or the config layer's
			// ControlPlaneDefinitionValues{...}, both of which decide what
			// reaches the API
			name := ""
			switch typed := literal.Type.(type) {
			case *ast.SelectorExpr:
				name = typed.Sel.Name
			case *ast.Ident:
				name = typed.Name
			}
			if name != "ControlPlaneDefinition" && name != "ControlPlaneDefinitionValues" {
				return true
			}

			setsTier, setsAuthEnabled := false, false
			for _, element := range literal.Elts {
				keyed, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := keyed.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch key.Name {
				case "Tier":
					setsTier = true
				case "AuthEnabled":
					setsAuthEnabled = true
				}
			}

			// a literal naming only the definition refers to one that exists,
			// which is a different thing from describing a new one: the
			// configuration a definition carries is what marks a construction,
			// and AuthEnabled is the one every such site sets
			if !setsAuthEnabled {
				return true
			}
			constructed++
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

// TestACloudInstallAtTheDefaultTierSaysSo covers #684's notice: an operator
// who meant production, and did not know it is no longer the default on a
// cloud provider, finds out as the install starts.
func TestACloudInstallAtTheDefaultTierSaysSo(t *testing.T) {
	for _, provider := range []string{
		v0.KubernetesRuntimeInfraProviderEKS,
		v0.KubernetesRuntimeInfraProviderGKE,
		v0.KubernetesRuntimeInfraProviderOKE,
	} {
		notice := DefaultTierNotice(provider)
		assert.Contains(t, notice, threeport.ControlPlaneTierDev, provider)
		assert.Contains(t, notice, "--tier "+threeport.ControlPlaneTierProd, provider)
	}

	assert.Empty(
		t, DefaultTierNotice(v0.KubernetesRuntimeInfraProviderKind),
		"kind runs only the development tier, so there is nothing to tell",
	)
}

// captureStdout returns what run prints to standard output.
func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	previous := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previous }()

	run()
	require.NoError(t, writer.Close())
	printed, err := io.ReadAll(reader)
	require.NoError(t, err)

	return string(printed)
}

// TestTheTierUpInstallsAtIsPrintedWhenDefaulted covers the path tptctl up
// takes to its tier: the notice is printed, not only composed, when the tier
// was defaulted on a cloud provider - and nothing is printed when the
// operator asked for a tier, or on kind.
func TestTheTierUpInstallsAtIsPrintedWhenDefaulted(t *testing.T) {
	var tier string
	printed := captureStdout(t, func() {
		tier = ResolveControlPlaneTier(v0.KubernetesRuntimeInfraProviderGKE, "")
	})
	assert.Equal(t, threeport.ControlPlaneTierDev, tier)
	assert.Contains(t, printed, DefaultTierNotice(v0.KubernetesRuntimeInfraProviderGKE))

	for _, explicit := range []string{threeport.ControlPlaneTierDev, threeport.ControlPlaneTierProd} {
		printed = captureStdout(t, func() {
			tier = ResolveControlPlaneTier(v0.KubernetesRuntimeInfraProviderEKS, explicit)
		})
		assert.Equal(t, explicit, tier, "a tier asked for is the one installed")
		assert.Empty(t, printed, "a tier asked for needs no notice")
	}

	printed = captureStdout(t, func() {
		tier = ResolveControlPlaneTier(v0.KubernetesRuntimeInfraProviderKind, "")
	})
	assert.Equal(t, threeport.ControlPlaneTierDev, tier)
	assert.Empty(t, printed, "kind runs only the development tier")
}
