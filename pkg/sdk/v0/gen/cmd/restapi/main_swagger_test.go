package restapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// generateRestApiMain writes a rest-api main for a project at modulePath into
// a scratch directory and returns it.
func generateRestApiMain(t *testing.T, module bool, modulePath string) string {
	t.Helper()

	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("internal", "version"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join("internal", "version", "version.txt"), []byte("v0.0.1\n"), 0600))

	generator := &gen.Generator{
		Module:              module,
		ModulePath:          modulePath,
		GlobalVersionConfig: gen.GlobalVersionConfig{Versions: []gen.VersionConfig{{VersionName: "v0"}}},
	}
	require.NoError(t, GenRestApiMain(generator, &sdk.SdkConfig{ModuleName: "Widget", ApiNamespace: "widget.example.com"}))

	generated, err := os.ReadFile(filepath.Join("cmd", "rest-api", "main_gen.go"))
	require.NoError(t, err)

	return string(generated)
}

// TestAModuleServesItsOwnSwaggerDocs covers what a module's API server serves
// at its docs path: its own docs package, under the path the core API
// proxies to it - not the core API's docs at the core API's path.
func TestAModuleServesItsOwnSwaggerDocs(t *testing.T) {
	generated := generateRestApiMain(t, true, "example.com/widget-module")

	assert.Contains(t, generated, `_ "example.com/widget-module/pkg/api-server/v0/docs"`)
	assert.NotContains(t, generated, `"github.com/threeport/threeport/pkg/api-server/v0/docs"`, "a module serving the core API's docs describes the wrong API")
	assert.Contains(t, generated, `e.GET(apiserver_v0.SwaggerPath+"/*", echoswagger.WrapHandler)`)
	assert.NotContains(t, generated, "SwaggerRoutes(e)", "the core API's docs path is not where the core API proxies a module's")
}

// TestTheCoreApiServesItsDocsAsBefore covers the core API, whose docs stay at
// swagger/index.html.
func TestTheCoreApiServesItsDocsAsBefore(t *testing.T) {
	generated := generateRestApiMain(t, false, "github.com/threeport/threeport")

	assert.Contains(t, generated, `_ "github.com/threeport/threeport/pkg/api-server/v0/docs"`)
	assert.Contains(t, generated, "routes_v0.SwaggerRoutes(e)")
	assert.NotContains(t, generated, "SwaggerPath")
}
