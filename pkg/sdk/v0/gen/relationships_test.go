package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexOf returns the position of name in names, or -1 when it is absent.
func indexOf(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

// sortFixture builds a Generator with only the foreign-key graph populated.
func sortFixture(dependencies map[string][]string) *Generator {
	return &Generator{RelationshipDependencies: dependencies}
}

// TestParseRelationshipDependencies_SkipsManyToMany covers many2many vs has-many edges.
func TestParseRelationshipDependencies_SkipsManyToMany(t *testing.T) {
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

	// many2many keys a join table; neither side holds a foreign key
	assert.NotContains(t, dependencies, "Left")
	assert.NotContains(t, dependencies, "Right")

	// has-many still puts the key on the child
	assert.Equal(t, []string{"Parent"}, dependencies["Child"])
}

// TestSortDatabaseInitNamesByDependency_ReferencedBeforeReferencing covers referenced-first order.
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

// TestSortDatabaseInitNamesByDependency_DropsDuplicateNames covers a repeated name.
func TestSortDatabaseInitNamesByDependency_DropsDuplicateNames(t *testing.T) {
	g := sortFixture(map[string][]string{
		"A": {"B"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"A", "B", "B"})

	assert.Equal(t, []string{"B", "A"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_TransitiveChain covers A->B->C order.
func TestSortDatabaseInitNamesByDependency_TransitiveChain(t *testing.T) {
	g := sortFixture(map[string][]string{
		"A": {"B"},
		"B": {"C"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"A", "B", "C"})

	assert.Equal(t, []string{"C", "B", "A"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_IgnoresExternalReference covers a name not in the list.
func TestSortDatabaseInitNamesByDependency_IgnoresExternalReference(t *testing.T) {
	g := sortFixture(map[string][]string{
		"Local": {"ExternalThing"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"Other", "Local"})

	assert.Equal(t, []string{"Local", "Other"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_Deterministic covers alphabetical ties.
func TestSortDatabaseInitNamesByDependency_Deterministic(t *testing.T) {
	g := sortFixture(map[string][]string{})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"Charlie", "Bravo", "Alpha"})

	assert.Equal(t, []string{"Alpha", "Bravo", "Charlie"}, sorted)
}

// TestSortDatabaseInitNamesByDependency_CycleFallback covers emitting every name in a cycle.
func TestSortDatabaseInitNamesByDependency_CycleFallback(t *testing.T) {
	g := sortFixture(map[string][]string{
		"Yin":  {"Yang"},
		"Yang": {"Yin"},
	})

	sorted := g.SortDatabaseInitNamesByDependency([]string{"Yin", "Yang"})

	assert.Equal(t, []string{"Yang", "Yin"}, sorted)
}

// TestValidateRelationshipCycles_AcyclicGraph covers a chain and a self-reference.
func TestValidateRelationshipCycles_AcyclicGraph(t *testing.T) {
	g := sortFixture(map[string][]string{
		"A":    {"B"},
		"B":    {"C"},
		"Self": {"Self"},
	})

	assert.NoError(t, g.ValidateRelationshipCycles())
}

// TestValidateRelationshipCycles_ReportsCycle covers a two-type cycle in the error.
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

// TestValidateRelationshipCycles_StableCycleReport covers a stable cycle report.
func TestValidateRelationshipCycles_StableCycleReport(t *testing.T) {
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
