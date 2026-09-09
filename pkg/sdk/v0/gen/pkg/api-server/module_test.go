package apiserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// moduleFixtureGenerator returns a generator with a module path outside
// the threeport project and one reconciled Widget group.
func moduleFixtureGenerator() *gen.Generator {
	return &gen.Generator{
		Module:     true,
		ModulePath: "example.com/widget-module",
		ApiObjectGroups: []gen.ApiObjectGroup{
			{
				ControllerDomain: "Widget",
				ControllerName:   "widget-controller",
				ReconciledObjects: []gen.ReconciledObject{
					{Name: "WidgetDefinition", Versions: []string{"v0"}},
					{Name: "WidgetInstance", Versions: []string{"v0"}},
				},
				ApiObjects: []*gen.ApiObject{
					{TypeName: "WidgetDefinition", Version: "v0", Reconciler: true},
					{TypeName: "WidgetInstance", Version: "v0", Reconciler: true},
				},
			},
		},
	}
}

// moduleFixtureSdkConfig returns an SDK config named Widget in the
// widget.example.com namespace.
func moduleFixtureSdkConfig() *sdk.SdkConfig {
	return &sdk.SdkConfig{
		ModuleName:   "Widget",
		ApiNamespace: "widget.example.com",
	}
}

// generateModuleRegistration writes module registration source into a
// scratch directory and returns it.
func generateModuleRegistration(t *testing.T) string {
	t.Helper()

	// get the process working directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read working directory: %v", err)
	}
	// change to a scratch directory so relative writes stay out of the source tree
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("failed to change to scratch directory: %v", err)
	}
	// restore the process working directory
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	})

	// generate the module registration source
	if err := GenModuleRegistration(moduleFixtureGenerator(), moduleFixtureSdkConfig()); err != nil {
		t.Fatalf("GenModuleRegistration returned an error: %v", err)
	}

	// read the generated module registration
	generated, err := os.ReadFile(filepath.Join("pkg", "api-server", "v0", "module_gen.go"))
	if err != nil {
		t.Fatalf("failed to read generated module registration: %v", err)
	}

	// return the generated source
	return string(generated)
}

// TestGenModuleRegistrationEmitsModuleName covers the module name the
// control plane registers the module under.
func TestGenModuleRegistrationEmitsModuleName(t *testing.T) {
	// generate the module registration source
	generated := generateModuleRegistration(t)

	// check the generated source declares the module name
	if want := `"widget.example.com/widget-module-api"`; !strings.Contains(generated, want) {
		t.Errorf("generated module registration does not declare module name %s", want)
	}
}

// TestGenModuleRegistrationImportsModuleRoutes covers the generated
// import of the module's own routes package.
func TestGenModuleRegistrationImportsModuleRoutes(t *testing.T) {
	// generate the module registration source
	generated := generateModuleRegistration(t)

	// check the generated source imports the module's own routes package
	if want := `"example.com/widget-module/pkg/api-server/v0/routes"`; !strings.Contains(generated, want) {
		t.Errorf("generated module registration does not import %s", want)
	}
}

// TestGenModuleRegistrationRegistersReconciledObjects covers the generated
// lookups for the controller and its objects.
func TestGenModuleRegistrationRegistersReconciledObjects(t *testing.T) {
	// generate the module registration source
	generated := generateModuleRegistration(t)

	// check the generated source looks up the controller and both objects
	for _, want := range []string{
		`fmt.Sprintf("name=%s&moduleapiid=%d", "widget-controller"`,
		`"name=WidgetDefinition&moduleapiid=%d"`,
		`"name=WidgetInstance&moduleapiid=%d"`,
	} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated module registration does not look up %s", want)
		}
	}
}

// TestGenModuleRegistrationHonorsExcludeFiles covers skipping the write
// when the output path is excluded.
func TestGenModuleRegistrationHonorsExcludeFiles(t *testing.T) {
	// get the process working directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read working directory: %v", err)
	}
	// change to a scratch directory so relative writes stay out of the source tree
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("failed to change to scratch directory: %v", err)
	}
	// restore the process working directory
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	})

	// exclude the generated registration file
	generatedPath := filepath.Join("pkg", "api-server", "v0", "module_gen.go")
	sdkConfig := moduleFixtureSdkConfig()
	sdkConfig.ExcludeFiles = []string{generatedPath}

	// generate the module registration source
	if err := GenModuleRegistration(moduleFixtureGenerator(), sdkConfig); err != nil {
		t.Fatalf("GenModuleRegistration returned an error: %v", err)
	}

	// check the excluded file was not written
	if _, err := os.Stat(generatedPath); !os.IsNotExist(err) {
		t.Errorf("excluded file %s was written anyway", generatedPath)
	}
}
