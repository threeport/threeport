package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A table created ahead of the one its foreign key references fails the
// migration, so a wrong order surfaces when the migration runs against
// CockroachDB, not when the code is generated. These tests cover the parsed
// graph, the cycle check, and the emitted order, none of them against a
// database.
//
// Three mechanisms decide what the assertions are worth. A many2many keys off a
// join table, so counting each side's slice as a has-many would invent a cycle.
// A name missing from the sorted list is a table the migration never creates. A
// cycle fails the generator run before the sort is reached, and a self-reference
// sits inside one table, so it needs no other table created first.
//
// Go randomizes the start of every range over a map, so the cycle walk sorts the
// type names, and each type's references, before descending.

// indexOf returns the position of name in names, or -1 when it is absent.
func indexOf(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

// sortFixture builds a Generator carrying only the dependency graph, the one
// field the relationship functions read. A key is the type whose table holds
// the foreign keys, and its value is the types those keys reference.
func sortFixture(dependencies map[string][]string) *Generator {
	return &Generator{RelationshipDependencies: dependencies}
}

// TestParseRelationshipDependencies_SkipsManyToMany asserts a many2many pair
// contributes no edge while a has-many pair still does.
func TestParseRelationshipDependencies_SkipsManyToMany(t *testing.T) {
	// build model source holding a many2many pair and a has-many pair
	const tick = "`"
	source := "package v0\n\n" +
		"type Left struct {\n" +
		"\tRights []*Right " + tick + `gorm:"many2many:lefts_rights;"` + tick + "\n" +
		"}\n\n" +
		"type Right struct {\n" +
		"\tLefts []*Left " + tick + `gorm:"many2many:lefts_rights;"` + tick + "\n" +
		"}\n\n" +
		"type Parent struct {\n" +
		"\tChildren []*Child\n" +
		"}\n\n" +
		"type Child struct {\n" +
		"\tParentID *uint\n" +
		"}\n"

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.go"), []byte(source), 0o600))

	dependencies, err := parseRelationshipDependencies(dir)
	require.NoError(t, err)

	assert.NotContains(t, dependencies, "Left")
	assert.NotContains(t, dependencies, "Right")

	assert.Equal(t, []string{"Parent"}, dependencies["Child"])
}

// TestSortDatabaseInitNamesByDependency_ReferencedBeforeReferencing asserts the
// referenced type is emitted ahead of the type whose table holds the key.
func TestSortDatabaseInitNamesByDependency_ReferencedBeforeReferencing(t *testing.T) {
	g := sortFixture(map[string][]string{
		"Child": {"Parent"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{
		"Child",
		"Parent",
	})

	parentIdx := indexOf(sorted, "Parent")
	childIdx := indexOf(sorted, "Child")
	require.NotEqual(t, -1, parentIdx)
	require.NotEqual(t, -1, childIdx)
	assert.Less(t, parentIdx, childIdx, "referenced table must precede referencing table")
}

// TestSortDatabaseInitNamesByDependency_DropsDuplicateNames asserts a repeated
// name is emitted once and nothing else is lost.
func TestSortDatabaseInitNamesByDependency_DropsDuplicateNames(t *testing.T) {
	g := sortFixture(map[string][]string{
		"A": {"B"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"A", "B", "B"})

	assert.Equal(t, []string{"B", "A"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_TransitiveChain asserts a three type
// chain is emitted from the end that references nothing.
func TestSortDatabaseInitNamesByDependency_TransitiveChain(t *testing.T) {
	g := sortFixture(map[string][]string{
		"A": {"B"},
		"B": {"C"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"A", "B", "C"})

	assert.Equal(t, []string{"C", "B", "A"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_IgnoresExternalReference asserts a
// reference to a name outside the list does not hold its referrer back.
func TestSortDatabaseInitNamesByDependency_IgnoresExternalReference(t *testing.T) {
	g := sortFixture(map[string][]string{
		"Local": {"ExternalThing"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"Other", "Local"})

	assert.Equal(t, []string{"Local", "Other"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_Deterministic asserts names with no
// references come out alphabetically rather than in the order given, so the
// generated migration does not reorder itself between runs.
func TestSortDatabaseInitNamesByDependency_Deterministic(t *testing.T) {
	g := sortFixture(map[string][]string{})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"Charlie", "Bravo", "Alpha"})

	assert.Equal(t, []string{"Alpha", "Bravo", "Charlie"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_CycleFallback asserts a cycle still
// yields every name, in name order, rather than dropping its members.
func TestSortDatabaseInitNamesByDependency_CycleFallback(t *testing.T) {
	g := sortFixture(map[string][]string{
		"Yin":  {"Yang"},
		"Yang": {"Yin"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"Yin", "Yang"})

	assert.Equal(t, []string{"Yang", "Yin"}, sorted)
}

// TestValidateRelationshipCycles_AcyclicGraph asserts a chain passes and a type
// referencing itself is accepted.
func TestValidateRelationshipCycles_AcyclicGraph(t *testing.T) {
	g := sortFixture(map[string][]string{
		"A":    {"B"},
		"B":    {"C"},
		"Self": {"Self"},
	})

	assert.NoError(t, g.ValidateRelationshipCycles())
}

// TestValidateRelationshipCycles_ReportsCycle asserts a two type cycle fails
// and names both types in the error.
func TestValidateRelationshipCycles_ReportsCycle(t *testing.T) {
	g := sortFixture(map[string][]string{
		"Yin":  {"Yang"},
		"Yang": {"Yin"},
	})

	err := g.ValidateRelationshipCycles()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Yin")
	assert.Contains(t, err.Error(), "Yang")
}

// TestValidateRelationshipCycles_StableCycleReport asserts a graph carrying two
// cycles reports the same one every run.
func TestValidateRelationshipCycles_StableCycleReport(t *testing.T) {
	// build a fresh graph per pass so nothing carries over between passes
	build := func() *Generator {
		return sortFixture(map[string][]string{
			"Yin":   {"Yang"},
			"Yang":  {"Yin"},
			"Alpha": {"Beta"},
			"Beta":  {"Alpha"},
		})
	}

	first := build().ValidateRelationshipCycles().Error()
	for i := 0; i < 25; i++ {
		assert.Equal(t, first, build().ValidateRelationshipCycles().Error(),
			"reported cycle must be stable across runs")
	}
}
