package provider

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFunctionsWithAContextDoNotDiscardIt covers the plumbing a deadline
// depends on.
//
// A function that takes a context and then derives its own from
// context.Background() throws away whatever bound its caller set, and nothing
// says so: it compiles, the parameter is used - by being shadowed - and the
// only symptom is a step that runs longer than the timeout around it allows.
// createEbsStorageAddon did exactly that, through a mechanical edit that
// treated it as an entry point when its caller already had a context to give.
//
// Deriving a deadline is for the functions that have no caller to inherit one
// from, which here are the ones taking no context at all.
func TestFunctionsWithAContextDoNotDiscardIt(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fileSet := token.NewFileSet()
	for _, name := range sources {
		parsed, err := parser.ParseFile(fileSet, name, nil, 0)
		require.NoError(t, err)

		ast.Inspect(parsed, func(node ast.Node) bool {
			function, ok := node.(*ast.FuncDecl)
			if !ok || function.Body == nil || !takesAContext(function) {
				return true
			}

			ast.Inspect(function.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok || !isSelector(call.Fun, "context", "Background") {
					return true
				}
				t.Errorf(
					"%s: %s takes a context and then derives one from context.Background(), "+
						"discarding its caller's deadline",
					fileSet.Position(call.Pos()), function.Name.Name,
				)

				return false
			})

			return true
		})
	}
}

// takesAContext reports whether a function declares a context.Context
// parameter.
func takesAContext(function *ast.FuncDecl) bool {
	if function.Type.Params == nil {
		return false
	}
	for _, param := range function.Type.Params.List {
		if isSelector(param.Type, "context", "Context") {
			return true
		}
	}

	return false
}

// isSelector reports whether an expression is pkg.name.
func isSelector(expr ast.Expr, pkg, name string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)

	return ok && identifier.Name == pkg
}
