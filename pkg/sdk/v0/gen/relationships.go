package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// The generated migration calls CreateTable once per model, so table order
// is the order of names we emit. A foreign key must point at a table that
// already exists.

// ValidateRelationshipCycles reports one cycle in the foreign-key graph.
// A self-reference is not a cycle: gorm puts that constraint on the table's
// own CREATE.
func (g *Generator) ValidateRelationshipCycles() error {
	typeNames := make([]string, 0, len(g.RelationshipDependencies))
	for typeName := range g.RelationshipDependencies {
		typeNames = append(typeNames, typeName)
	}
	sort.Strings(typeNames)

	visiting := make(map[string]bool, len(typeNames))
	visited := make(map[string]bool, len(typeNames))
	var path []string

	var walk func(typeName string) []string
	walk = func(typeName string) []string {
		if visiting[typeName] {
			start := 0
			for i, name := range path {
				if name == typeName {
					start = i
					break
				}
			}
			return append(append([]string{}, path[start:]...), typeName)
		}
		if visited[typeName] {
			return nil
		}

		visiting[typeName] = true
		path = append(path, typeName)

		referenced := append([]string{}, g.RelationshipDependencies[typeName]...)
		sort.Strings(referenced)
		for _, next := range referenced {
			if next == typeName {
				continue
			}
			if cycle := walk(next); cycle != nil {
				return cycle
			}
		}

		path = path[:len(path)-1]
		visiting[typeName] = false
		visited[typeName] = true
		return nil
	}

	for _, typeName := range typeNames {
		if cycle := walk(typeName); cycle != nil {
			return fmt.Errorf(
				"circular foreign-key dependency between API types: %s",
				strings.Join(cycle, " -> "),
			)
		}
	}

	return nil
}

// SortDatabaseInitNamesByDependency puts referenced types before the types
// that hold a key into them. Duplicates are dropped; ties break alphabetically.
func (g *Generator) SortDatabaseInitNamesByDependency(names []string) []string {
	// one CreateTable per name
	seen := make(map[string]bool, len(names))
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		unique = append(unique, name)
	}
	names = unique

	inList := make(map[string]bool, len(names))
	for _, name := range names {
		inList[name] = true
	}

	// edges that point at another name in this list
	dependsOn := make(map[string]map[string]bool, len(names))
	for _, name := range names {
		dependsOn[name] = make(map[string]bool)
		for _, referenced := range g.RelationshipDependencies[name] {
			if inList[referenced] && referenced != name {
				dependsOn[name][referenced] = true
			}
		}
	}

	emitted := make(map[string]bool, len(names))
	sorted := make([]string, 0, len(names))
	for len(sorted) < len(names) {
		// names whose referenced tables are already out
		var ready []string
		for _, name := range names {
			if emitted[name] {
				continue
			}
			allDepsEmitted := true
			for dep := range dependsOn[name] {
				if !emitted[dep] {
					allDepsEmitted = false
					break
				}
			}
			if allDepsEmitted {
				ready = append(ready, name)
			}
		}
		if len(ready) == 0 {
			// cycle: emit remaining names alphabetically
			var remaining []string
			for _, name := range names {
				if !emitted[name] {
					remaining = append(remaining, name)
				}
			}
			sort.Strings(remaining)
			sorted = append(sorted, remaining...)
			break
		}
		sort.Strings(ready)
		next := ready[0]
		sorted = append(sorted, next)
		emitted[next] = true
	}

	return sorted
}

// parseRelationshipDependencies reads foreign keys from API model source.
// The generator does not import those packages, so it cannot reflect on them.
func parseRelationshipDependencies(dir string) (map[string][]string, error) {
	structs, err := parseModelStructs(dir)
	if err != nil {
		return nil, err
	}

	modelNames := make(map[string]bool, len(structs))
	for name := range structs {
		modelNames[name] = true
	}

	dependencies := map[string][]string{}
	for typeName, structType := range structs {
		// ID columns on this struct, used to confirm a belongs-to
		keyFields := make(map[string]bool)
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if strings.HasSuffix(name.Name, "ID") {
					keyFields[name.Name] = true
				}
			}
		}

		for _, field := range structType.Fields.List {
			if len(field.Names) == 0 {
				continue
			}

			if joinTableAssociation(field) {
				continue
			}

			// has-many: the child's table holds the key
			if child, ok := sliceElementModel(field.Type, modelNames); ok {
				dependencies[child] = append(dependencies[child], typeName)
				continue
			}

			// belongs-to: this table holds TypeNameID
			if referenced, ok := singularModel(field.Type, modelNames); ok {
				if keyFields[referenced+"ID"] {
					dependencies[typeName] = append(dependencies[typeName], referenced)
				}
			}
		}
	}

	return dependencies, nil
}

// parseModelStructs returns every package-level struct in dir, keyed by name.
func parseModelStructs(dir string) (map[string]*ast.StructType, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*ast.StructType{}, nil
		}
		return nil, fmt.Errorf("failed to read model directory %s: %w", dir, err)
	}

	structs := map[string]*ast.StructType{}
	for _, entry := range entries {
		fileName := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(fileName, ".go") {
			continue
		}
		if strings.HasSuffix(fileName, "_gen.go") ||
			strings.HasSuffix(fileName, "_test.go") ||
			strings.HasSuffix(fileName, "_validate.go") {
			continue
		}

		filePath := filepath.Join(dir, fileName)
		fset := token.NewFileSet()
		parsedFile, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
		if err != nil {
			return nil, fmt.Errorf("failed to parse model file %s: %w", filePath, err)
		}

		for _, decl := range parsedFile.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				structs[typeSpec.Name.Name] = structType
			}
		}
	}

	return structs, nil
}

// joinTableAssociation reports whether the field carries a gorm many2many
// setting, which puts the association's keys in a join table.
func joinTableAssociation(field *ast.Field) bool {
	if field.Tag == nil {
		return false
	}
	gormTag := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("gorm")
	// match on the setting key alone; the value is the join table name
	for _, setting := range strings.Split(gormTag, ";") {
		key, _, _ := strings.Cut(setting, ":")
		if strings.EqualFold(strings.TrimSpace(key), "many2many") {
			return true
		}
	}
	return false
}

// sliceElementModel returns the model type name the element of a []Model or
// []*Model field names, and whether that element type is a model at all. A
// pointer to a slice does not match.
func sliceElementModel(expr ast.Expr, modelNames map[string]bool) (string, bool) {
	arrayType, ok := expr.(*ast.ArrayType)
	if !ok || arrayType.Len != nil {
		return "", false
	}
	if name, ok := identModel(arrayType.Elt, modelNames); ok {
		return name, true
	}
	return "", false
}

// singularModel returns the name of the model a non-slice field names, and
// whether that type is a model at all.
func singularModel(expr ast.Expr, modelNames map[string]bool) (string, bool) {
	return identModel(expr, modelNames)
}

// identModel returns the model type name a bare identifier or a pointer to one
// names. A type qualified by another package's name never matches.
func identModel(expr ast.Expr, modelNames map[string]bool) (string, bool) {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return "", false
	}
	if modelNames[ident.Name] {
		return ident.Name, true
	}
	return "", false
}
