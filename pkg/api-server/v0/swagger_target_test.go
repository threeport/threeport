package v0_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOneTargetRegeneratesTheSwaggerDocs covers how many ways there are to
// regenerate the committed docs, and which binary does it.
//
// Two targets did: the hand-written GenerateDocs, and GenerateSwaggerDocs
// emitted by the SDK generator. A second target named as though it were the
// authoritative one is not a harmless duplicate - whichever is reached for has
// to agree with the committed output, and the generated one resolved swag from
// $PATH rather than the version go.mod pins. Whether that agrees is a property
// of the developer's machine, so a disagreement reads as the doc pipeline being
// broken rather than as the wrong binary having run.
//
// TestSwaggerDocsIncludeAPIFields checks what the docs contain; this checks
// there is one way to produce them.
func TestOneTargetRegeneratesTheSwaggerDocs(t *testing.T) {
	t.Chdir(moduleRoot(t))

	magefiles, err := filepath.Glob(filepath.Join("magefiles", "*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, magefiles)

	fileSet := token.NewFileSet()
	var commands []string
	for _, path := range magefiles {
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || render(call.Fun) != "exec.Command" {
				return true
			}
			arguments := stringArguments(call)
			if !slicesContain(arguments, "swag") {
				return true
			}
			commands = append(commands, path)

			// the pinned version is reached through the go.mod tool
			// directive; a bare command is whatever the developer happens to
			// have installed, which is why this reads the arguments rather
			// than the source text - the same command written on one line
			// says nothing different
			assert.Equal(
				t, []string{"go", "tool", "swag"}, firstThree(arguments),
				"%s:%d resolves swag from $PATH instead of the version go.mod pins",
				path, fileSet.Position(call.Pos()).Line,
			)

			return true
		})
	}

	assert.Len(
		t, commands, 1,
		"the swagger docs are regenerated one way, by %s", strings.Join(commands, " and "),
	)
}

// stringArguments returns a call's string literal arguments, in order, skipping
// any argument that is not one.
func stringArguments(call *ast.CallExpr) []string {
	var arguments []string
	for _, argument := range call.Args {
		literal, ok := argument.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		if value, err := strconv.Unquote(literal.Value); err == nil {
			arguments = append(arguments, value)
		}
	}

	return arguments
}

func firstThree(values []string) []string {
	if len(values) < 3 {
		return values
	}

	return values[:3]
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

// render prints an expression the way the source spells it.
func render(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return render(typed.X) + "." + typed.Sel.Name
	}

	return ""
}
