package installer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// generateModuleInstaller writes a module's installer and its database
// policy test into a scratch directory and returns their source.
func generateModuleInstaller(t *testing.T) (installerSource, testSource string) {
	t.Helper()

	t.Chdir(t.TempDir())
	generator := &gen.Generator{
		Module:     true,
		ModulePath: "example.com/widget-module",
		ApiObjectGroups: []gen.ApiObjectGroup{{
			ControllerDomain: "Widget",
			ControllerName:   "widget-controller",
			ReconciledObjects: []gen.ReconciledObject{
				{Name: "WidgetInstance", Versions: []string{"v0"}},
			},
			ApiObjects: []*gen.ApiObject{
				{TypeName: "WidgetInstance", Version: "v0", Reconciler: true},
			},
		}},
	}
	sdkConfig := &sdk.SdkConfig{ModuleName: "Widget", ApiNamespace: "widget.example.com"}
	require.NoError(t, GenInstaller(generator, sdkConfig))
	require.NoError(t, GenInstallerDatabasePolicyTest(generator, sdkConfig))

	read := func(name string) string {
		source, err := os.ReadFile(filepath.Join("pkg", "installer", "v0", name))
		require.NoError(t, err)

		return string(source)
	}

	return read("installer_gen.go"), read("database_policy_gen_test.go")
}

// mapLiteral returns the keys of a map literal, each with the source of its
// value.
func mapLiteral(t *testing.T, literal *ast.CompositeLit) map[string]string {
	t.Helper()

	entries := map[string]string{}
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		require.True(t, ok, "expected a keyed map literal")
		key, ok := keyed.Key.(*ast.BasicLit)
		require.True(t, ok, "expected a literal key")
		name, err := strconv.Unquote(key.Value)
		require.NoError(t, err)
		switch value := keyed.Value.(type) {
		case *ast.Ident:
			entries[name] = value.Name
		case *ast.BasicLit:
			entries[name] = value.Value
		default:
			entries[name] = "<expression>"
		}
	}

	return entries
}

// field returns the value of a key in a map literal.
func field(literal *ast.CompositeLit, key string) ast.Expr {
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if name, ok := keyed.Key.(*ast.BasicLit); ok && name.Value == strconv.Quote(key) {
			return keyed.Value
		}
	}

	return nil
}

// nested follows keys through nested map literals.
func nested(literal *ast.CompositeLit, keys ...string) *ast.CompositeLit {
	for _, key := range keys {
		next, ok := field(literal, key).(*ast.CompositeLit)
		if !ok {
			return nil
		}
		literal = next
	}

	return literal
}

// TestTheDatabasePolicyAdmitsThePodsTheApiServerRunsIn covers what the
// generated test cannot: that the labels the policy admits are the ones the
// module's API server deployment gives its pods and its service selects.
// The generated test builds the policy and its expectation from the same
// labels, so a deployment whose labels changed would pass it, and then
// install a module whose db-init cannot reach its database.
func TestTheDatabasePolicyAdmitsThePodsTheApiServerRunsIn(t *testing.T) {
	installerSource, _ := generateModuleInstaller(t)
	file, err := parser.ParseFile(token.NewFileSet(), "installer_gen.go", installerSource, 0)
	require.NoError(t, err)

	var policyLabels, podLabels, serviceSelector map[string]string
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			if typed.Name.Name == "apiServerPodLabels" {
				returned := typed.Body.List[0].(*ast.ReturnStmt).Results[0].(*ast.CompositeLit)
				policyLabels = mapLiteral(t, returned)
			}
		case *ast.ValueSpec:
			if len(typed.Names) != 1 || len(typed.Values) != 1 {
				return true
			}
			object := unstructuredObject(typed.Values[0])
			if object == nil {
				return true
			}
			switch typed.Names[0].Name {
			case "widgetApiDeploy":
				if labels := nested(object, "spec", "template", "metadata", "labels"); labels != nil {
					podLabels = mapLiteral(t, labels)
				}
			case "widgetApiService":
				if selector := nested(object, "spec", "selector"); selector != nil {
					serviceSelector = mapLiteral(t, selector)
				}
			}
		}

		return true
	})

	want := map[string]string{"app.kubernetes.io/name": "apiServerName"}
	assert.Equal(t, want, policyLabels, "the labels the policy admits")
	assert.Equal(t, policyLabels, podLabels, "the API server deployment's pods must carry the labels the policy admits")
	assert.Equal(t, policyLabels, serviceSelector, "reinstall admits what the API server's service selects, which must be the same pods")
}

// unstructuredObject returns the Object map literal of an
// &unstructured.Unstructured{Object: ...} expression.
func unstructuredObject(expr ast.Expr) *ast.CompositeLit {
	unary, ok := expr.(*ast.UnaryExpr)
	if !ok {
		return nil
	}
	literal, ok := unary.X.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := keyed.Key.(*ast.Ident); ok && key.Name == "Object" {
			object, _ := keyed.Value.(*ast.CompositeLit)
			return object
		}
	}

	return nil
}

// TestTheDatabasePolicyIsInstalledBeforeTheApiServer covers the order: the
// API server's db-init connects to the database as soon as its pod starts,
// so the policy that admits it has to be there first, and the namespace that
// owns the policy has to exist before it.
func TestTheDatabasePolicyIsInstalledBeforeTheApiServer(t *testing.T) {
	installerSource, _ := generateModuleInstaller(t)

	namespace := strings.Index(installerSource, "kube.CreateOrUpdateResource(\n\t\tnamespace,")
	policy := strings.Index(installerSource, "tp_installer.InstallModuleDatabasePolicy(")
	deployment := strings.Index(installerSource, "kube.CreateOrUpdateResource(widgetApiDeploy,")
	require.NotEqual(t, -1, namespace, "the namespace is not created")
	require.NotEqual(t, -1, policy, "the database policy is not installed")
	require.NotEqual(t, -1, deployment, "the API server is not deployed")
	assert.Less(t, namespace, policy, "the policy is owned by the namespace, which has to exist first")
	assert.Less(t, policy, deployment, "the API server's db-init would start before the policy admitting it")

	assert.Contains(t, installerSource, `tp_installer.InstallModuleDatabasePolicy(
		i.KubeClient,
		*i.KubeRestMapper,
		i.ThreeportNamespace,
		i.ModuleNamespace,
		apiServerName,
		apiServerPodLabels(),
	)`)
	assert.NotContains(t, installerSource, "threeport.io/module", "a module is admitted by its namespace's name, not a label anything can add")
}

// TestEveryModuleTestsItsDatabasePolicy covers the generated test itself.
func TestEveryModuleTestsItsDatabasePolicy(t *testing.T) {
	_, testSource := generateModuleInstaller(t)

	assert.Contains(t, testSource, "func TestOnlyTheModuleApiServerReachesTheDatabase(t *testing.T)")
	assert.Contains(t, testSource, "tp_installer.ModuleDatabasePolicy(")
	assert.Contains(t, testSource, `networkpolicytest.AssertModuleDatabasePolicy(
		t,
		policy,
		defaultThreeportNamespace,
		defaultNamespace,
		apiServerPodLabels(),
	)`)
}

// TestAModuleKeepingItsInstallerGetsNoPolicyTest covers a module that
// excludes the generated installer to keep its own: the generated test calls
// apiServerPodLabels, which that installer does not have, so writing the
// test would leave the module's installer package failing to compile.
func TestAModuleKeepingItsInstallerGetsNoPolicyTest(t *testing.T) {
	t.Chdir(t.TempDir())
	testFile := filepath.Join("pkg", "installer", "v0", "database_policy_gen_test.go")

	require.NoError(t, GenInstallerDatabasePolicyTest(
		&gen.Generator{Module: true, ModulePath: "example.com/widget-module"},
		&sdk.SdkConfig{ModuleName: "Widget", ExcludeFiles: []string{filepath.Join("pkg", "installer", "v0", "installer_gen.go")}},
	))
	_, err := os.Stat(testFile)
	assert.True(t, os.IsNotExist(err), "the policy test is written though the installer it calls is excluded")

	require.NoError(t, GenInstallerDatabasePolicyTest(
		&gen.Generator{Module: true, ModulePath: "example.com/widget-module"},
		&sdk.SdkConfig{ModuleName: "Widget"},
	))
	_, err = os.Stat(testFile)
	assert.NoError(t, err, "the policy test is written when the installer is generated")
}
