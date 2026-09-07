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

// The generated database migration creates one table per gorm CreateTable
// call, and gorm writes a relationship's foreign key constraint into the
// CREATE TABLE statement of the table that holds the key, so the referenced
// table has to exist by then. gorm reorders the models handed to one
// CreateTable call, but the generated migration passes a single model per call,
// which leaves nothing to reorder, so the order the generator emits names in is
// the order the tables are created in. The migration runs in an init container
// on the API server, so an order the database rejects surfaces at control plane
// install rather than at generation.
//
// The dependency graph is parsed out of the API model source rather than read
// from gorm, because the generator is a separate binary that never links the
// generated project's packages and so has no types to reflect on. An edge
// points from the type whose table holds a foreign key to the type it
// references. A bare ID column with no association field beside it carries no
// constraint and no edge. A referenced type marked ExcludeFromDb has no table,
// so nothing waits on it and it carries no edge either.

// ValidateRelationshipCycles returns an error naming one cycle in the foreign
// key graph, or nil when the graph is acyclic. A type that references itself is
// not a cycle, since gorm writes that constraint into the table's own CREATE
// TABLE statement.
func (g *Generator) ValidateRelationshipCycles() error {
	// walk the types in name order, and each type's references in name order,
	// so a repeat run reports the same cycle
	typeNames := make([]string, 0, len(g.RelationshipDependencies))
	for typeName := range g.RelationshipDependencies {
		typeNames = append(typeNames, typeName)
	}
	sort.Strings(typeNames)

	visiting := make(map[string]bool, len(typeNames))
	visited := make(map[string]bool, len(typeNames))
	var path []string

	// walk returns the cycle it closes, or nil when the branch is clean
	var walk func(typeName string) []string
	walk = func(typeName string) []string {
		// reaching a type already on the walk stack closes a cycle
		if visiting[typeName] {
			// report the cycle alone, trimmed back to where this type first
			// appeared, not the walk that reached it
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
			// a key pointing back into the same table is one table, not a cycle
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

// SortDatabaseInitNamesByDependency orders names so that every referenced type
// comes before the types referencing it. Duplicates are dropped, references to
// names outside the list are ignored, and ties break alphabetically.
func (g *Generator) SortDatabaseInitNamesByDependency(names []string) []string {
	// drop duplicate names so each table is created once
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

	// keep only the edges that point at another name being sorted here
	dependsOn := make(map[string]map[string]bool, len(names))
	for _, name := range names {
		dependsOn[name] = make(map[string]bool)
		for _, referenced := range g.RelationshipDependencies[name] {
			if inList[referenced] && referenced != name {
				dependsOn[name][referenced] = true
			}
		}
	}

	// emit the alphabetically first name whose references are already out,
	// one name per pass
	emitted := make(map[string]bool, len(names))
	sorted := make([]string, 0, len(names))
	for len(sorted) < len(names) {
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
		// nothing is ready, so the names left form a cycle; emit them in name
		// order to keep the output stable, since ValidateRelationshipCycles
		// already fails the run on a real one
		if len(ready) == 0 {
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

// parseRelationshipDependencies reads the API model source in dir and returns,
// for each type, the names of the types its table holds a foreign key into.
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
		// collect the ID columns so an association field can be matched to the
		// key that carries it
		keyFields := make(map[string]bool)
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if strings.HasSuffix(name.Name, "ID") {
					keyFields[name.Name] = true
				}
			}
		}

		for _, field := range structType.Fields.List {
			// skip anonymous embeds, which carry no field name
			if len(field.Names) == 0 {
				continue
			}

			// a many2many association keys off a join table, so neither side
			// holds a key into the other
			if joinTableAssociation(field) {
				continue
			}

			// a slice of a model is a has-many, and the key sits on the
			// element type
			if child, ok := sliceElementModel(field.Type, modelNames); ok {
				dependencies[child] = append(dependencies[child], typeName)
				continue
			}

			// a singular model field counts as a key on this table only when
			// the struct also declares a field named for the referenced type
			// plus ID; gorm names that key after the field rather than the
			// type, so a field whose name differs from its type is not matched
			if referenced, ok := singularModel(field.Type, modelNames); ok {
				if keyFields[referenced+"ID"] {
					dependencies[typeName] = append(dependencies[typeName], referenced)
				}
			}
		}
	}

	return dependencies, nil
}

// parseModelStructs returns every package level struct type declared in the
// model source in dir, keyed by type name. A project without that directory
// gets an empty map.
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
		// skip the generated, validation and test source that sits beside the
		// models; a test fixture struct would otherwise count as a model
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

		// record every top level struct type by name
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
