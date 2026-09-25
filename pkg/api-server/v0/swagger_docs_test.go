package v0_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestSwaggerDocsIncludeAPIFields checks that every published pkg/api struct
// keeps its exported fields in the swagger definitions.
func TestSwaggerDocsIncludeAPIFields(t *testing.T) {
	structs := apiStructs(t)
	defs := swaggerDefinitions(t)

	var missing []string
	for name, def := range defs {
		structName, ok := strings.CutPrefix(name, "v0.")
		if !ok {
			continue
		}
		st, ok := structs[structName]
		if !ok {
			continue
		}
		for _, field := range swaggerFields(structs, st, map[string]bool{}) {
			if _, ok := def.Properties[field]; !ok {
				missing = append(missing, structName+"."+field)
			}
		}
	}
	require.Empty(t, missing)
}

type swaggerDef struct {
	Properties map[string]yaml.Node `yaml:"properties"`
}

func swaggerDefinitions(t *testing.T) map[string]swaggerDef {
	t.Helper()
	body, err := os.ReadFile("docs/swagger.yaml")
	require.NoError(t, err)
	var doc struct {
		Definitions map[string]swaggerDef `yaml:"definitions"`
	}
	require.NoError(t, yaml.Unmarshal(body, &doc))
	return doc.Definitions
}

func apiStructs(t *testing.T) map[string]*ast.StructType {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "..", "api", "v0"), func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)
	structs := map[string]*ast.StructType{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts := spec.(*ast.TypeSpec)
					st, ok := ts.Type.(*ast.StructType)
					if !ok || !ast.IsExported(ts.Name.Name) {
						continue
					}
					structs[ts.Name.Name] = st
				}
			}
		}
	}
	return structs
}

// swaggerFields returns the property names swag emits for a struct, including
// fields promoted from an embedded struct.
func swaggerFields(structs map[string]*ast.StructType, st *ast.StructType, seen map[string]bool) []string {
	var fields []string
	if st.Fields == nil {
		return fields
	}
	for _, field := range st.Fields.List {
		tag := ""
		if field.Tag != nil {
			tag = strings.Trim(field.Tag.Value, "`")
		}
		if reflect.StructTag(tag).Get("swaggerignore") == "true" {
			continue
		}
		if jsonName, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ","); jsonName == "-" {
			continue
		}
		if len(field.Names) == 0 {
			embName := embeddedStructName(field.Type)
			if embName == "" || seen[embName] {
				continue
			}
			emb, ok := structs[embName]
			if !ok {
				continue
			}
			seen[embName] = true
			fields = append(fields, swaggerFields(structs, emb, seen)...)
			continue
		}
		for _, name := range field.Names {
			if ast.IsExported(name.Name) {
				fields = append(fields, name.Name)
			}
		}
	}
	return fields
}

func embeddedStructName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return embeddedStructName(e.X)
	default:
		return ""
	}
}
