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

// TestGenModuleRegistrationReclaimsControllerAtomically is a regression test
// for threeport#575, a follow-up to threeport#558. #558 was a ModuleController
// name conflict resolved by rebinding the existing row without checking its
// owner. The fix for that checked the owner with GetModuleApiByID and then
// rebound with UpdateModuleController, two separate round-trips, so the owner
// could change between the check and the write.
//
// The generated upsertModuleController must resolve the conflict through the
// single ReclaimModuleController call, which checks and rebinds in one server
// transaction. It must not read or write the conflicting row itself, because
// any of those calls reopens the gap.
func TestGenModuleRegistrationReclaimsControllerAtomically(t *testing.T) {
	// generate the module registration source
	generated := generateModuleRegistration(t)

	funcBody := upsertModuleControllerFuncPattern.FindString(generated)
	if funcBody == "" {
		t.Fatalf("could not find upsertModuleController function body in generated source:\n%s", generated)
	}

	// a plain create is still tried first, so the common path is unchanged
	createIdx := strings.Index(funcBody, "tp_client.CreateModuleController(")
	reclaimIdx := strings.Index(funcBody, "tp_client.ReclaimModuleController(")
	if createIdx == -1 {
		t.Fatalf("upsertModuleController does not try a plain create first:\n%s", funcBody)
	}
	if reclaimIdx == -1 {
		t.Fatalf("upsertModuleController does not reclaim through ReclaimModuleController:\n%s", funcBody)
	}
	if createIdx >= reclaimIdx {
		t.Errorf("the plain create must run before the reclaim call:\n%s", funcBody)
	}

	// the reclaim runs only when the create reports a name conflict
	if !regexp.MustCompile(`errors\.Is\(createErr,\s*tp_client_lib\.ErrConflict\)`).MatchString(funcBody) {
		t.Errorf("upsertModuleController does not limit the reclaim to a name conflict:\n%s", funcBody)
	}

	// no separate check or write against the conflicting row
	for _, call := range []string{
		"tp_client.GetModuleApiByID(",
		"tp_client.GetModuleControllerByName(",
		"tp_client.UpdateModuleController(",
		"tp_client.ReplaceModuleController(",
	} {
		if strings.Contains(funcBody, call) {
			t.Errorf(
				"upsertModuleController calls %s; the owner check and the reclaim must be one server-side "+
					"transaction, not separate round-trips:\n%s",
				call, funcBody,
			)
		}
	}

	// a failed reclaim, including a 409 for an active owner, must not be swallowed
	failureBranch := regexp.MustCompile(`(?s)if reclaimErr != nil \{(.*?)\n\t\}`).FindStringSubmatch(funcBody)
	if failureBranch == nil {
		t.Fatalf("could not find the \"reclaimErr != nil\" branch in upsertModuleController:\n%s", funcBody)
	}
	if !strings.Contains(failureBranch[1], "return nil,") || !strings.Contains(failureBranch[1], "%w") {
		t.Errorf("a failed reclaim must return a wrapped error, got:\n%s", failureBranch[1])
	}

	// a name collision with an active module is the case that needs operator
	// action, so a conflict must log before returning (threeport#576) - the
	// module crash loops on restart and the error alone can get buried in the
	// crash-loop noise. The server also returns a 409 for a transient
	// concurrent change, so the log must carry the server's error rather than
	// assert a permanent collision.
	conflictBranch := regexp.MustCompile(`(?s)if errors\.Is\(reclaimErr,\s*tp_client_lib\.ErrConflict\) \{(.*?)\n\t\t\}`).FindStringSubmatch(failureBranch[1])
	if conflictBranch == nil {
		t.Fatalf("could not find the \"errors.Is(reclaimErr, ErrConflict)\" (active owner) branch in the reclaim failure path:\n%s", failureBranch[1])
	}
	if !strings.Contains(conflictBranch[1], "log.Printf(") {
		t.Errorf("the active-owner collision branch must log before the error is returned, got:\n%s", conflictBranch[1])
	}
	if !strings.Contains(conflictBranch[1], "reclaimErr") {
		t.Errorf("the collision log must include the server's reclaim error to tell an active owner from a concurrent change, got:\n%s", conflictBranch[1])
	}
	if strings.Contains(conflictBranch[1], "return") {
		t.Errorf("the collision log branch must fall through to the wrapped error return, got:\n%s", conflictBranch[1])
	}
}
