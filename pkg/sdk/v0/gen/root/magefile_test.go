package root

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// generatedBuildFunctions runs GenMagefile() in a temporary directory and
// returns the source of each function declared on the Build type, keyed by
// function name.
func generatedBuildFunctions(t *testing.T) map[string]string {
	// generate into a temporary directory because the output path is relative
	t.Chdir(t.TempDir())
	require.NoError(t, GenMagefile(
		&gen.Generator{ModulePath: "example.com/project"},
		&sdk.SdkConfig{},
	))
	generatedPath := filepath.Join("magefiles", "magefile_gen.go")
	source, err := os.ReadFile(generatedPath)
	require.NoError(t, err)

	// index the source of every method on the Build type
	parsed, err := parser.ParseFile(token.NewFileSet(), generatedPath, source, 0)
	require.NoError(t, err)
	functions := map[string]string{}
	for _, decl := range parsed.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Recv == nil {
			continue
		}
		receiver, ok := function.Recv.List[0].Type.(*ast.Ident)
		if !ok || receiver.Name != "Build" {
			continue
		}
		start := function.Pos() - parsed.FileStart
		end := function.End() - parsed.FileStart
		functions[function.Name.Name] = string(source[start:end])
	}
	return functions
}

// TestAllImagesAppliesTheImagesSelectionToCompileAndPackage covers that the
// generated AllImages filters both the binaries it compiles and the images it
// packages, so a filtered run cannot compile one set and package another.
func TestAllImagesAppliesTheImagesSelectionToCompileAndPackage(t *testing.T) {
	body := generatedBuildFunctions(t)["AllImages"]
	require.NotEmpty(t, body, "the generator no longer emits AllImages")

	// the selection comes from the IMAGES environment variable
	assert.Contains(t, body, `os.Getenv("IMAGES")`)

	// each component is filtered once before compiling and once before packaging
	compileStep := strings.Index(body, "util.BuildBinaries(")
	require.NotEqual(t, -1, compileStep, "AllImages no longer compiles binaries")
	for _, binaryName := range []string{"rest-api", "database-migrator", "agent"} {
		check := `util.ImageSelected(selection, "` + binaryName + `")`
		assert.Equal(t, 2, strings.Count(body, check), "AllImages has to filter %s for compile and package", binaryName)
		assert.Less(t, strings.Index(body, check), compileStep, "the compile list for %s has to be filtered before compiling", binaryName)
		assert.Greater(t, strings.LastIndex(body, check), compileStep, "the package tasks for %s have to be filtered after compiling", binaryName)
	}
}

// TestAllImagesRejectsUnknownNamesBeforeBuilding covers that a selection
// naming no known component fails the run before any binary compiles.
func TestAllImagesRejectsUnknownNamesBeforeBuilding(t *testing.T) {
	body := generatedBuildFunctions(t)["AllImages"]
	require.NotEmpty(t, body, "the generator no longer emits AllImages")

	// the unknown-name check and the empty-selection check both come first
	compileStep := strings.Index(body, "util.BuildBinaries(")
	require.NotEqual(t, -1, compileStep, "AllImages no longer compiles binaries")
	unknownCheck := strings.Index(body, "util.UnknownImages(selection,")
	emptyCheck := strings.Index(body, "len(packageDirs) == 0")
	require.NotEqual(t, -1, unknownCheck, "AllImages no longer checks for unknown names")
	require.NotEqual(t, -1, emptyCheck, "AllImages no longer checks for an empty selection")
	assert.Less(t, unknownCheck, compileStep, "unknown names have to fail before compiling")
	assert.Less(t, emptyCheck, compileStep, "an empty selection has to fail before compiling")
}

// TestAllImagesDevIgnoresTheImagesSelection covers that the development image
// target builds every component whatever IMAGES holds.
func TestAllImagesDevIgnoresTheImagesSelection(t *testing.T) {
	body := generatedBuildFunctions(t)["AllImagesDev"]
	require.NotEmpty(t, body, "the generator no longer emits AllImagesDev")

	// no part of the selection reaches the development target
	for _, selectionUse := range []string{"IMAGES", "ImageSelected", "UnknownImages", "selection"} {
		assert.NotContains(t, body, selectionUse)
	}
}
