package apiserver

import (
	"os"
	"path/filepath"
	"regexp"
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

// upsertModuleControllerFuncPattern isolates the generated
// upsertModuleController function body: from its signature to the next
// line starting with a bare "}" - the closing brace gofmt puts at column 0
// for a top-level function.
var upsertModuleControllerFuncPattern = regexp.MustCompile(
	`(?s)func upsertModuleController\(.*?\n\}`,
)

// TestGenModuleRegistrationGuardsControllerReclaim is a regression test for
// threeport#558: a ModuleController name conflict was resolved by
// unconditionally rebinding the existing row to the registering module,
// without checking whether the row's current owner (ModuleApiID) was a
// still-active ModuleApi or an orphaned one left behind by a deleted
// module. That let one module silently steal a still-active module's
// controller registration by picking the same controller name.
//
// This checks more than "the right substrings appear somewhere in the
// file" - a loose Contains check would pass even if the ownership check
// were on the wrong variable, or placed after the rebind instead of before
// it. It isolates the upsertModuleController function body and asserts the
// GetModuleApiByID ownership check appears strictly before the
// existing.ModuleApiID reassignment within it.
func TestGenModuleRegistrationGuardsControllerReclaim(t *testing.T) {
	// generate the module registration source
	generated := generateModuleRegistration(t)

	funcBody := upsertModuleControllerFuncPattern.FindString(generated)
	if funcBody == "" {
		t.Fatalf("could not find upsertModuleController function body in generated source:\n%s", generated)
	}

	ownershipCheckIdx := strings.Index(funcBody, "tp_client.GetModuleApiByID(")
	if ownershipCheckIdx == -1 {
		t.Fatalf("upsertModuleController does not check the conflicting row's owner via GetModuleApiByID:\n%s", funcBody)
	}

	if !regexp.MustCompile(`errors\.Is\(getErr,\s*tp_client_lib\.ErrObjectNotFound\)`).MatchString(funcBody) {
		t.Errorf("upsertModuleController does not distinguish an orphaned owner (ErrObjectNotFound) from a still-active one:\n%s", funcBody)
	}

	rebindIdx := strings.Index(funcBody, "existing.ModuleApiID = moduleApiID")
	if rebindIdx == -1 {
		t.Fatalf("upsertModuleController no longer reassigns existing.ModuleApiID - generator output may have changed shape:\n%s", funcBody)
	}

	if ownershipCheckIdx >= rebindIdx {
		t.Errorf(
			"the GetModuleApiByID ownership check must run before the existing.ModuleApiID reassignment, "+
				"or a conflicting row owned by a still-active module api would be rebound before its owner is checked:\n%s",
			funcBody,
		)
	}
}
