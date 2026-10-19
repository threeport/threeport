package provider

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryResourceManagerRoleIsTaggedAsOurs guards the only thing that makes
// adopting an existing role safe.
//
// createRoleIfAbsent refuses a role whose tags are not the ones this install
// would have given it, so what the caller passes is the whole ownership
// signal. A caller passing tpaws.IamTags(name, map[string]string{}) asks for
// nothing but a generic Name tag - a convention half the tooling in an AWS
// account follows - and the check then admits a role threeport never created.
//
// This cannot be reached by a unit test: CreateResourceManagerRole builds its
// own IAM client from an aws.Config. The call sites are few and the mistake is
// invisible at a glance, so the source is checked instead.
func TestEveryResourceManagerRoleIsTaggedAsOurs(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	callSites := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "vendor", "node_modules":
				return fs.SkipDir
			}

			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(source), "CreateResourceManagerRole(") {
			return nil
		}

		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, source, 0)
		if err != nil {
			return err
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			if calledName(call.Fun) != "CreateResourceManagerRole" {
				return true
			}

			callSites++
			where := fileSet.Position(call.Pos())
			tags := render(fileSet, source, call.Args[1])
			assert.Contains(
				t, tags, "ThreeportProviderTags",
				"%s passes tags that do not carry the ProvisionedBy marker, so an existing role "+
					"created by anything else would be adopted instead of refused; got %s",
				where, tags,
			)

			return true
		})

		return nil
	})
	require.NoError(t, err)

	assert.NotZero(
		t, callSites,
		"this test found nothing to check, which means it has stopped guarding anything",
	)
}

// calledName returns the name of the function a call expression calls, whether
// it is called plainly or through a package.
func calledName(fun ast.Expr) string {
	switch called := fun.(type) {
	case *ast.Ident:
		return called.Name
	case *ast.SelectorExpr:
		return called.Sel.Name
	}

	return ""
}

// render returns the source text of an expression.
func render(fileSet *token.FileSet, source []byte, expr ast.Expr) string {
	start := fileSet.Position(expr.Pos()).Offset
	end := fileSet.Position(expr.End()).Offset

	return string(source[start:end])
}
