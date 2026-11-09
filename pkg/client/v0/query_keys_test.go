package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// queryKeyPattern matches the key in each `key=` of a query string literal.
var queryKeyPattern = regexp.MustCompile(`(?:^|&)([^=&]+)=`)

// TestQueryStringKeysAreBinderSpelling guards the keys of every query string
// handed to a Get...ByQueryString client call, and of the
// ...Query helpers that build them. The API server's query binder
// accepts only the lowercased field name and answers any other spelling, such
// as a hyphenated key, with a 400. A mismatch there strands whatever called it
// (a delete that never completes, for one), so fail it here instead.
func TestQueryStringKeysAreBinderSpelling(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)

	var bad []string
	checked := 0
	fset := token.NewFileSet()
	for _, dir := range []string{"internal", "pkg", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}
			// check every string literal under node for query keys
			checkLiterals := func(node ast.Node) {
				ast.Inspect(node, func(n ast.Node) bool {
					lit, ok := n.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return true
					}
					val, err := strconv.Unquote(lit.Value)
					if err != nil {
						return true
					}
					for _, m := range queryKeyPattern.FindAllStringSubmatch(val, -1) {
						checked++
						if m[1] != strings.ToLower(m[1]) || strings.Contains(m[1], "-") {
							rel, _ := filepath.Rel(root, path)
							bad = append(bad, rel+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+" key "+strconv.Quote(m[1]))
						}
					}
					return true
				})
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					// the query string is the last argument; checking every
					// literal in it covers Sprintf formats too
					if len(n.Args) > 0 && isQueryStringCall(n) {
						checkLiterals(n.Args[len(n.Args)-1])
					}
				case *ast.FuncDecl:
					// helpers that build a query string for a call, such as
					// sharedNamespaceQuery, keep the literal out of the call
					if n.Body != nil && strings.HasSuffix(n.Name.Name, "Query") {
						checkLiterals(n.Body)
					}
				}
				return true
			})
			return nil
		})
		require.NoError(t, err)
	}

	require.NotZero(t, checked, "found no query string keys; the check is not looking where the calls are")
	require.Empty(t, bad,
		"query keys must be the lowercased field name, e.g. kubernetesruntimeinstanceid; the API server rejects anything else with a 400")
}

// isQueryStringCall reports whether call is a client Get...ByQueryString call.
func isQueryStringCall(call *ast.CallExpr) bool {
	var name string
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		name = fn.Sel.Name
	case *ast.Ident:
		name = fn.Name
	}
	return strings.HasPrefix(name, "Get") && strings.HasSuffix(name, "ByQueryString")
}
