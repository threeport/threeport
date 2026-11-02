package root

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
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
