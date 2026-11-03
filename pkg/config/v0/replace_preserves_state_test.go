package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stillClearsControllerState lists the Replace implementations that have not
// been fixed yet, from threeport#565.
//
// Richard asked for this to be triaged per type rather than landed as one
// change, so the list is the triage checklist, enforced: a type missing from it
// that clears the fields fails the test, and fixing one means deleting its line.
// It may only shrink.
var stillClearsControllerState = map[string][]string{
	"AwsEksKubernetesRuntimeInstance": {"Reconciled", "Status"},
	"ControlPlaneDefinition":          {"Reconciled"},
	"ControlPlaneInstance":            {"Reconciled", "Status"},
	"DomainNameDefinition":            {"Reconciled"},
	"DomainNameInstance":              {"Reconciled", "Status"},
	"GatewayDefinition":               {"Reconciled"},
	"GatewayInstance":                 {"Reconciled", "Status"},
	"GcpGkeKubernetesRuntimeInstance": {"Reconciled", "Status"},
	"HelmWorkloadDefinition":          {"Reconciled"},
	"HelmWorkloadInstance":            {"Reconciled", "Status"},
	"KubernetesRuntimeDefinition":     {"Reconciled"},
	"KubernetesRuntimeInstance":       {"Reconciled", "Status"},
	"KubernetesWorkloadDefinition":    {"Reconciled"},
	"MachineRuntimeInstance":          {"Reconciled", "Status"},
	"ObservabilityStackDefinition":    {"Reconciled"},
	"ObservabilityStackInstance":      {"Reconciled", "Status"},
	"OciOkeKubernetesRuntimeInstance": {"Reconciled", "Status"},
	"SecretDefinition":                {"Reconciled"},
	"SecretInstance":                  {"Reconciled", "Status"},
	"TerraformDefinition":             {"Reconciled"},
}

// TestReplacePreservesControllerState covers what a Replace payload leaves out.
//
// Replace sends a PUT, which writes every field on the object - a nil one
// included. A field left off the outgoing literal is therefore not "leave this
// alone" but "set this to nothing", so every reconciler-owned field a Replace
// forgets is silently cleared on an unrelated edit. Nothing about writing one
// of these by hand says so, which is why nearly all of them have the gap.
//
// Status is the easier one to miss: most types take it from the embedded
// Instance struct rather than declaring it, so a literal writing Instance with
// only a Name clears it without ever naming the field.
func TestReplacePreservesControllerState(t *testing.T) {
	fileSet := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	var regressed []string
	fixed := map[string]bool{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Name.Name != "Replace" || function.Recv == nil {
				continue
			}
			typeName := strings.TrimSuffix(receiverType(function), "Config")
			if !isReconcilable(typeName) {
				continue
			}

			cleared := clearedFields(function, typeName)
			known := stillClearsControllerState[typeName]
			for _, field := range cleared {
				if !contains(known, field) {
					regressed = append(regressed, typeName+" clears "+field)
				}
			}
			for _, field := range known {
				if !contains(cleared, field) {
					fixed[typeName+" no longer clears "+field] = true
				}
			}
		}
	}

	sort.Strings(regressed)
	assert.Empty(
		t, regressed,
		"a Replace payload must carry the reconciler's fields over from the object it fetched",
	)

	var stale []string
	for entry := range fixed {
		stale = append(stale, entry)
	}
	sort.Strings(stale)
	assert.Empty(
		t, stale,
		"remove these from stillClearsControllerState - the list may only shrink",
	)
}

// clearedFields returns the reconciler-owned fields this Replace's outgoing
// payload leaves out, and so clears.
func clearedFields(function *ast.FuncDecl, typeName string) []string {
	var cleared []string

	payload := payloadLiteral(function, typeName)
	if payload == nil {
		// no literal found: report both, so an unreadable Replace shows up
		// rather than passing quietly
		return []string{"Reconciled", "Status"}
	}

	if typeEmbeds(typeName, "Reconciliation") && !setsReconciled(payload) {
		cleared = append(cleared, "Reconciled")
	}
	if typeHasStatus(typeName) && !setsStatus(payload) {
		cleared = append(cleared, "Status")
	}

	return cleared
}

// payloadLiteral finds the api object literal a Replace sends.
func payloadLiteral(function *ast.FuncDecl, typeName string) *ast.CompositeLit {
	var found *ast.CompositeLit
	ast.Inspect(function, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != typeName {
			return true
		}
		found = literal

		return false
	})

	return found
}

// setsStatus reports whether a payload carries Status, whether the type
// declares its own or takes the one promoted from the embedded Instance.
func setsStatus(payload *ast.CompositeLit) bool {
	return setsField(payload, "Status") || setsNestedField(payload, "Instance", "Status")
}

// setsReconciled reports whether a payload carries Reconciled.
//
// Naming the Reconciliation struct is not the same as setting the field in it:
// an empty literal writes the zero value, which is what clears the saved
// progress. The field is also writable directly, since it is promoted, so both
// spellings count.
func setsReconciled(payload *ast.CompositeLit) bool {
	return setsField(payload, "Reconciled") || setsNestedField(payload, "Reconciliation", "Reconciled")
}

// setsNestedField reports whether a payload sets inner inside an outer struct
// literal it names.
func setsNestedField(literal *ast.CompositeLit, outer, inner string) bool {
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := keyed.Key.(*ast.Ident)
		if !ok || key.Name != outer {
			continue
		}
		if nested, ok := keyed.Value.(*ast.CompositeLit); ok {
			return setsField(nested, inner)
		}
	}

	return false
}

func setsField(literal *ast.CompositeLit, name string) bool {
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := keyed.Key.(*ast.Ident); ok && key.Name == name {
			return true
		}
	}

	return false
}

func receiverType(function *ast.FuncDecl) string {
	if len(function.Recv.List) == 0 {
		return ""
	}
	switch typed := function.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := typed.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return typed.Name
	}

	return ""
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

// apiType records what an API object carries, read from the source rather than
// by reflection: the point is what a literal in this package has to name, and
// a promoted field is invisible to a check that only looks at the type.
type apiType struct {
	embeds map[string]bool
	fields map[string]bool
}

var apiTypes = loadApiTypes()

func loadApiTypes() map[string]apiType {
	types := map[string]apiType{}
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, filepath.Join("..", "..", "api", "v0"), nil, 0)
	if err != nil {
		return types
	}

	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.TypeSpec)
				if !ok {
					return true
				}
				structType, ok := spec.Type.(*ast.StructType)
				if !ok {
					return true
				}
				recorded := apiType{embeds: map[string]bool{}, fields: map[string]bool{}}
				for _, field := range structType.Fields.List {
					if len(field.Names) == 0 {
						if ident, ok := field.Type.(*ast.Ident); ok {
							recorded.embeds[ident.Name] = true
						}
						continue
					}
					for _, name := range field.Names {
						recorded.fields[name.Name] = true
					}
				}
				types[spec.Name.Name] = recorded

				return true
			})
		}
	}

	return types
}

func typeEmbeds(typeName, embedded string) bool {
	return apiTypes[typeName].embeds[embedded]
}

// typeHasStatus reports whether a type carries Status at all - declared on
// itself, or promoted from the Instance struct it embeds.
func typeHasStatus(typeName string) bool {
	recorded, ok := apiTypes[typeName]
	if !ok {
		return false
	}
	if recorded.fields["Status"] {
		return true
	}

	return recorded.embeds["Instance"] && apiTypes["Instance"].fields["Status"]
}

// isReconcilable reports whether a type has any controller-owned state a
// replace could clear.
func isReconcilable(typeName string) bool {
	if _, ok := apiTypes[typeName]; !ok {
		return false
	}

	return typeEmbeds(typeName, "Reconciliation") || typeHasStatus(typeName)
}

// TestClearedFieldsSeesAnEmptyStructLiteral covers the matcher the checklist
// rests on, since one that silently matches makes the whole list pass.
//
// Both fields live inside an embedded struct, so naming that struct is not the
// same as setting the field in it: an empty literal writes the zero value,
// which is exactly what clears the saved value on a PUT.
func TestClearedFieldsSeesAnEmptyStructLiteral(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		payload string
		cleared []string
	}{
		{
			name:    "both carried over",
			payload: `Instance: api_v0.Instance{Name: n, Status: existing.Status}, Reconciliation: api_v0.Reconciliation{Reconciled: existing.Reconciled}`,
		},
		{
			name:    "an empty Reconciliation literal clears it",
			payload: `Instance: api_v0.Instance{Name: n, Status: existing.Status}, Reconciliation: api_v0.Reconciliation{}`,
			cleared: []string{"Reconciled"},
		},
		{
			name:    "an Instance literal with only a Name clears Status",
			payload: `Instance: api_v0.Instance{Name: n}, Reconciliation: api_v0.Reconciliation{Reconciled: existing.Reconciled}`,
			cleared: []string{"Status"},
		},
		{
			name:    "neither named at all",
			payload: `Instance: api_v0.Instance{Name: n}`,
			cleared: []string{"Reconciled", "Status"},
		},
		{
			name:    "promoted fields written directly",
			payload: `Name: n, Status: existing.Status, Reconciled: existing.Reconciled`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			source := "package p\nfunc (c *TerraformInstanceConfig) Replace() { x := &api.TerraformInstance{" +
				testCase.payload + "} ; _ = x }"
			file, err := parser.ParseFile(token.NewFileSet(), "payload.go", source, 0)
			require.NoError(t, err)
			function, ok := file.Decls[0].(*ast.FuncDecl)
			require.True(t, ok)

			cleared := clearedFields(function, "TerraformInstance")

			sort.Strings(cleared)
			assert.Equal(t, testCase.cleared, nilIfEmpty(cleared))
		})
	}
}

func nilIfEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	return values
}
