package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// TestDeletingANamespaceThatIsAlreadyGoneSucceeds covers the retry of a
// teardown whose namespace was deleted after the first attempt stopped
// waiting for it: there is nothing left to delete, and reporting that as a
// failure would fail every retry until the caller gave up.
func TestDeletingANamespaceThatIsAlreadyGoneSucceeds(t *testing.T) {
	kubeClient := testKubeClient()
	mapper := testNamespaceMapper()

	require.NoError(t, DeleteNamespaces(kubeClient, &mapper, []string{"released"}))
}

// TestDeletingANamespaceStillReportsOtherFailures keeps the tolerance above to
// a namespace that is gone: a delete the API server refuses is still a failure.
func TestDeletingANamespaceStillReportsOtherFailures(t *testing.T) {
	kubeClient := testKubeClient(testNamespace("released", ""))
	kubeClient.PrependReactor("delete", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	mapper := testNamespaceMapper()

	err := DeleteNamespaces(kubeClient, &mapper, []string{"released"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "released")
}

// TestEveryCallerOfDeleteNamespacesUsesItsResult covers #706: the helm
// workload controller called DeleteNamespaces as a bare statement, so a
// namespace that was not deleted - and the volumes in it - went unreported
// and was never retried. Every call in the repository has to use the error,
// whichever package makes it.
func TestEveryCallerOfDeleteNamespacesUsesItsResult(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	calls := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch statement := node.(type) {
			case *ast.ExprStmt:
				if callsDeleteNamespaces(statement.X) {
					calls++
					t.Errorf("%s discards the error from DeleteNamespaces", fileSet.Position(statement.Pos()))
				}
			case *ast.AssignStmt:
				for _, value := range statement.Rhs {
					if !callsDeleteNamespaces(value) {
						continue
					}
					calls++
					if ident, ok := statement.Lhs[0].(*ast.Ident); ok && ident.Name == "_" {
						t.Errorf("%s discards the error from DeleteNamespaces", fileSet.Position(statement.Pos()))
					}
				}
			case *ast.ReturnStmt:
				for _, value := range statement.Results {
					if callsDeleteNamespaces(value) {
						calls++
					}
				}
			}
			return true
		})

		return nil
	})
	require.NoError(t, err)

	// the control plane uninstall and the helm workload controller
	assert.GreaterOrEqual(t, calls, 2, "the walk found fewer callers than exist, so it is not looking where they are")
}

// callsDeleteNamespaces reports whether an expression is a call to
// DeleteNamespaces, by package selector or from inside this package.
func callsDeleteNamespaces(expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch function := call.Fun.(type) {
	case *ast.SelectorExpr:
		return function.Sel.Name == "DeleteNamespaces"
	case *ast.Ident:
		return function.Name == "DeleteNamespaces"
	}

	return false
}
