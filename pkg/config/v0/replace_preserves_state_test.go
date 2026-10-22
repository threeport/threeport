package v0

import (
	"fmt"
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

// TestEveryReplaceStartsFromTheExistingObject covers what a Replace payload
// is built from.
//
// Replace sends a PUT, which writes every field on the object - a nil one
// included. A payload assembled as a fresh literal therefore clears every
// field it does not name, which is how nearly every Replace here used to wipe
// the reconciler's progress, a runtime's connection or a control plane's
// certificates on an unrelated edit (threeport#565). Starting from a copy of
// the object the Replace fetched keeps them, so this checks that every Replace
// does, and that none sets the reconciler's fields itself - the copy already
// carries them.
func TestEveryReplaceStartsFromTheExistingObject(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fileSet := token.NewFileSet()
	var problems []string
	checked := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		require.NoError(t, err)

		fileProblems, fileChecked := replaceProblems(file, reconcilerFields(t))
		problems = append(problems, fileProblems...)
		checked += fileChecked
	}

	require.NotZero(t, checked, "no Replace sending an object was found, so nothing was checked")
	t.Logf("checked %d Replace methods sending an object", checked)
	sort.Strings(problems)
	assert.Empty(t, problems)
}

// reconcilerFields returns the fields a Replace must leave to the copy: the
// ones in Reconciliation, read from the API types, and Status.
func reconcilerFields(t *testing.T) map[string]bool {
	t.Helper()

	packages, err := parser.ParseDir(token.NewFileSet(), filepath.Join("..", "..", "api", "v0"), nil, 0)
	require.NoError(t, err)

	fields := map[string]bool{"Status": true}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.TypeSpec)
				if !ok || spec.Name.Name != "Reconciliation" {
					return true
				}
				for _, field := range spec.Type.(*ast.StructType).Fields.List {
					for _, name := range field.Names {
						fields[name.Name] = true
					}
				}

				return false
			})
		}
	}
	require.Greater(t, len(fields), 1, "the Reconciliation struct was not found in the API types")

	return fields
}

// replaceProblems reports each Replace method in file that does not build the
// object it sends from a copy of the one it fetched, and returns how many
// Replace methods sending an object it checked. A Replace that sends nothing
// itself has to call another Replace, or it is reported as unreadable rather
// than passing quietly.
func replaceProblems(file *ast.File, reconciler map[string]bool) ([]string, int) {
	var problems []string
	checked := 0
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Replace" || function.Recv == nil {
			continue
		}
		name := receiverType(function)

		call, typeName := clientReplaceCall(function)
		if call == nil {
			if !callsAnotherReplace(function) {
				problems = append(problems, name+": sends no object and calls no other Replace")
			}
			continue
		}
		checked++

		payload, ok := call.Args[len(call.Args)-1].(*ast.Ident)
		if !ok {
			problems = append(problems, name+": the payload sent is not a variable")
			continue
		}
		if source := copiedFrom(function, payload.Name); source == "" {
			problems = append(problems, fmt.Sprintf("%s: %s is not built as util.Ptr(*existing...)", name, payload.Name))
		} else if !fetched(function, source, typeName) {
			problems = append(problems, fmt.Sprintf("%s: %s copies %s, which is not the %s it fetched", name, payload.Name, source, typeName))
		}

		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.CompositeLit:
				if selector, ok := typed.Type.(*ast.SelectorExpr); ok && selector.Sel.Name == typeName {
					problems = append(problems, fmt.Sprintf("%s: builds a %s literal, which clears what it does not name", name, typeName))
				}
			case *ast.AssignStmt:
				for _, lhs := range typed.Lhs {
					selector, ok := lhs.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == payload.Name && reconciler[selector.Sel.Name] {
						problems = append(problems, fmt.Sprintf("%s: sets %s, which is the reconciler's", name, selector.Sel.Name))
					}
				}
			}

			return true
		})
	}

	return problems, checked
}

// clientReplaceCall finds the client call that sends the object, such as
// client_v0.ReplaceTerraformInstance, and returns it with the type it sends.
func clientReplaceCall(function *ast.FuncDecl) (*ast.CallExpr, string) {
	var found *ast.CallExpr
	var typeName string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !strings.HasPrefix(selector.Sel.Name, "Replace") || selector.Sel.Name == "Replace" {
			return true
		}
		if pkg, ok := selector.X.(*ast.Ident); ok && strings.HasPrefix(pkg.Name, "client") {
			found = call
			typeName = strings.TrimPrefix(selector.Sel.Name, "Replace")
		}

		return true
	})

	return found, typeName
}

// callsAnotherReplace reports whether a Replace hands off to another one, as
// the definition and instance pairs do.
func callsAnotherReplace(function *ast.FuncDecl) bool {
	calls := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok && selector.Sel.Name == "Replace" {
			calls = true
		}

		return !calls
	})

	return calls
}

// copiedFrom returns the variable a payload is declared as a copy of, from
// `payload := util.Ptr(*source)`, or "".
func copiedFrom(function *ast.FuncDecl, payload string) string {
	source := ""
	ast.Inspect(function.Body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); !ok || ident.Name != payload {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return false
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); !ok || selector.Sel.Name != "Ptr" {
			return false
		}
		star, ok := call.Args[0].(*ast.StarExpr)
		if !ok {
			return false
		}
		if ident, ok := star.X.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "existing") {
			source = ident.Name
		}

		return false
	})

	return source
}

// fetched reports whether source holds the object the Replace fetched by
// name, from `source, err := client_v0.Get<typeName>ByName(...)`.
func fetched(function *ast.FuncDecl, source, typeName string) bool {
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) == 0 || len(assign.Rhs) != 1 {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); !ok || ident.Name != source {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Get"+typeName+"ByName" {
			found = true
		}

		return !found
	})

	return found
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

// TestReplaceProblemsSeesEachWayToGetItWrong covers the matcher the check
// above rests on, since one that silently matches makes it pass on anything.
func TestReplaceProblemsSeesEachWayToGetItWrong(t *testing.T) {
	const fetch = "existing, err := client_v0.GetTerraformInstanceByName(c, e, n)\n_ = err\n"
	const send = "_, _ = client_v0.ReplaceTerraformInstance(c, e, updated)\n"
	reconciler := map[string]bool{"Reconciled": true, "Status": true}

	for _, testCase := range []struct {
		name    string
		body    string
		problem string
	}{
		{
			name: "a copy of the fetched object passes",
			body: fetch + "updated := util.Ptr(*existing)\nupdated.Name = n\n" + send,
		},
		{
			name:    "a literal clears what it does not name",
			body:    fetch + "updated := &api.TerraformInstance{Instance: api.Instance{Name: n}}\n" + send,
			problem: "is not built as util.Ptr(*existing...)",
		},
		{
			name:    "setting the reconciler's fields",
			body:    fetch + "updated := util.Ptr(*existing)\nupdated.Status = nil\n" + send,
			problem: "sets Status, which is the reconciler's",
		},
		{
			name:    "a copy of something other than the fetched object",
			body:    fetch + "existingDefinition, _ := client_v0.GetTerraformDefinitionByName(c, e, n)\nupdated := util.Ptr(*existingDefinition)\n" + send,
			problem: "which is not the TerraformInstance it fetched",
		},
		{
			name:    "a literal built anywhere in the Replace",
			body:    fetch + "updated := util.Ptr(*existing)\nother := &api.TerraformInstance{}\n_ = other\n" + send,
			problem: "builds a TerraformInstance literal",
		},
		{
			name:    "a Replace that sends nothing and hands off to nothing",
			body:    "_ = n\n",
			problem: "sends no object and calls no other Replace",
		},
		{
			name: "a Replace handing off to the definition and instance",
			body: "_ = definition.Replace(c, e, n)\n",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			source := "package p\nfunc (t *TerraformInstanceConfig) Replace() {\n" + testCase.body + "}\n"
			file, err := parser.ParseFile(token.NewFileSet(), "replace.go", source, 0)
			require.NoError(t, err)

			problems, _ := replaceProblems(file, reconciler)
			if testCase.problem == "" {
				assert.Empty(t, problems)
				return
			}
			require.NotEmpty(t, problems)
			assert.Contains(t, strings.Join(problems, "\n"), testCase.problem)
		})
	}
}
