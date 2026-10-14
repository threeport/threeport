package gateway

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reconcilerFunc matches the three entry points a reconciler has per type.
var reconcilerFunc = regexp.MustCompile(`^v\d+\w+?(Created|Updated|Deleted)$`)

// TestReconcilerLogsNameWhatTheyLog covers the two halves of a log line that a
// reader filters on, both of which are copied along with the line.
//
// These functions are written by copying a sibling, and a copied log keeps the
// original's wording and variable: the gateway definition's update path
// announced "created" and logged the associated workload definition's ID under
// gatewayDefinitionID. Filtering by that key returns the wrong object, and the
// event names the wrong operation - worse than an imprecise message, because
// both read as deliberate.
//
// The key names the object whose ID is logged, so "<name>ID" is paired with
// <name>.ID and nothing else. A success message ends in the operation its
// function performs.
func TestReconcilerLogsNameWhatTheyLog(t *testing.T) {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", nil, 0)
	require.NoError(t, err)

	for _, pkg := range packages {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			for _, decl := range file.Decls {
				function, ok := decl.(*ast.FuncDecl)
				if !ok || !reconcilerFunc.MatchString(function.Name.Name) {
					continue
				}
				operation := strings.ToLower(reconcilerFunc.FindStringSubmatch(function.Name.Name)[1])

				ast.Inspect(function, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || !isLogInfoCall(call) || len(call.Args) == 0 {
						return true
					}
					where := fileSet.Position(call.Pos())

					message, ok := stringLiteral(call.Args[0])
					require.True(t, ok, "%s: the log message is not a literal", where)

					// only the success logs claim an operation; the rest
					// report a decision taken along the way
					for _, verb := range []string{"created", "updated", "deleted"} {
						if strings.HasSuffix(message, verb) {
							assert.Equal(
								t, operation, verb,
								"%s: %s logs %q", where, function.Name.Name, message,
							)
						}
					}

					for i := 1; i+1 < len(call.Args); i += 2 {
						key, ok := stringLiteral(call.Args[i])
						if !ok || !strings.HasSuffix(key, "ID") {
							continue
						}
						assert.Equal(
							t, strings.TrimSuffix(key, "ID")+".ID", render(call.Args[i+1]),
							"%s: %s logs something else under %q", where, function.Name.Name, key,
						)
					}

					return true
				})
			}
		}
	}
}

// isLogInfoCall reports whether a call is one of the logger's Info calls,
// whether or not a verbosity or any other option was set on it.
//
// The bare logger counts as much as a chained one. Matching only the chained
// form would let dropping a .V(1) take the checks below off a log line along
// with it, which is the opposite of what a test written against copied code
// should do.
func isLogInfoCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Info" {
		return false
	}
	receiver := render(selector.X)

	return receiver == "log" || strings.HasPrefix(receiver, "log.")
}

// TestIsLogInfoCallMatchesEitherForm covers the matcher the checks above rest
// on, since a matcher that silently matches nothing makes them pass.
func TestIsLogInfoCallMatchesEitherForm(t *testing.T) {
	for _, testCase := range []struct {
		source  string
		matches bool
	}{
		{source: `log.Info("plain")`, matches: true},
		{source: `log.V(1).Info("with a verbosity")`, matches: true},
		{source: `log.V(1).WithValues("k", v).Info("with values")`, matches: true},
		{source: `log.Error(err, "not an info call")`},
		{source: `fmt.Println("not a log call")`},
		{source: `logger.Info("a different logger")`},
	} {
		t.Run(testCase.source, func(t *testing.T) {
			expr, err := parser.ParseExpr(testCase.source)
			require.NoError(t, err)
			call, ok := expr.(*ast.CallExpr)
			require.True(t, ok)

			assert.Equal(t, testCase.matches, isLogInfoCall(call))
		})
	}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)

	return value, err == nil
}

// render prints an expression the way the source spells it, for the dotted
// names these logs are written with.
func render(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return render(typed.X) + "." + typed.Sel.Name
	case *ast.CallExpr:
		return render(typed.Fun) + "()"
	}

	return ""
}
