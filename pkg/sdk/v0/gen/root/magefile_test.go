package root

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// TestTheSwagVersionModulesAreToldToUseIsThreeports covers the version in the
// message a module's GenerateSwaggerDocs gives when swag is not one of its go
// tools, and in the SDK tutorial. Both reach every module author, so they have
// to follow the version threeport's own go.mod pins rather than be bumped by
// hand alongside it.
func TestTheSwagVersionModulesAreToldToUseIsThreeports(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "go.mod"))
	require.NoError(t, err)
	goMod, err := modfile.Parse("go.mod", content, nil)
	require.NoError(t, err)

	var pinned string
	for _, required := range goMod.Require {
		if required.Mod.Path == "github.com/swaggo/swag" {
			pinned = required.Mod.Version
		}
	}
	require.NotEmpty(t, pinned, "go.mod no longer requires swag")
	assert.Equal(t, pinned, SwagVersion, "SwagVersion has to follow the swag version go.mod pins")

	var tools []string
	for _, tool := range goMod.Tool {
		tools = append(tools, tool.Path)
	}
	assert.True(t, slices.Contains(tools, swagToolPackage), "go.mod has to declare %s as a tool for GenerateSwaggerDocs to run it", swagToolPackage)

	// the SDK tutorial has module authors declare the same version
	tutorial, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "docs", "docs", "sdk", "tutorial.md"))
	require.NoError(t, err)
	assert.Contains(t, string(tutorial), "go get -tool "+swagToolPackage+"@"+SwagVersion, "the SDK tutorial has to tell module authors to declare the swag version threeport pins")
}

// TestAModuleBuildsItsPluginBinaryAndPluginImage covers a module's magefile:
// it has targets for the tptctl plugin binary a developer installs, and for the
// plugin image the module install job runs. The image is named plugin, not
// tptctl, because the job runs the module's own plugin command.
func TestAModuleBuildsItsPluginBinaryAndPluginImage(t *testing.T) {
	// generate a module's magefile into a scratch directory
	t.Chdir(t.TempDir())
	require.NoError(t, GenMagefile(
		&gen.Generator{Module: true, ModulePath: "example.com/widget-module"},
		&sdk.SdkConfig{ModuleName: "Widget", ApiNamespace: "widget.example.com"},
	))
	generated, err := os.ReadFile(filepath.Join("magefiles", "magefile_gen.go"))
	require.NoError(t, err)
	source := string(generated)

	// assert the plugin binary targets exist
	assert.Contains(t, source, "func (Build) TptctlBin(")
	assert.Contains(t, source, "func (Build) TptctlBinDev()")

	// assert the plugin image targets and name exist
	assert.Contains(t, source, "func (Build) PluginImage(")
	assert.Contains(t, source, "func (Build) PluginImageDev()")
	assert.Contains(t, source, "threeport-widget-plugin")

	// assert no tptctl image name remains
	assert.NotContains(t, source, "TptctlImage")
	assert.NotContains(t, source, "threeport-widget-tptctl")
}
