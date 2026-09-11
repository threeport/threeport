package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestBoundEventFilterClauseEmpty covers an empty filter producing no SQL.
func TestBoundEventFilterClauseEmpty(t *testing.T) {
	clause, values := boundEventFilterClause(&v0.Event{})

	assert.Equal(t, "", clause)
	assert.Empty(t, values)
}

// TestBoundEventFilterClauseColumns covers each bindable field mapping to
// its column and a single bound value.
func TestBoundEventFilterClauseColumns(t *testing.T) {
	tests := []struct {
		name   string
		filter v0.Event
		column string
		value  interface{}
	}{
		{"note", v0.Event{Note: util.Ptr("boom")}, "note", "boom"},
		{"count", v0.Event{Count: util.Ptr(uint(3))}, "count", uint(3)},
		{"type", v0.Event{Type: util.Ptr("Warning")}, "type", "Warning"},
		{
			"reporting controller",
			v0.Event{ReportingController: util.Ptr("kubernetes-workload-controller")},
			"reporting_controller",
			"kubernetes-workload-controller",
		},
		{
			"object type",
			v0.Event{ObjectType: util.Ptr("threeport.io/v0.KubernetesWorkloadInstance")},
			"object_type",
			"threeport.io/v0.KubernetesWorkloadInstance",
		},
		{"object id", v0.Event{ObjectID: util.Ptr(uint(42))}, "object_id", uint(42)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clause, values := boundEventFilterClause(&test.filter)

			assert.Equal(t, " AND v0_events."+test.column+" = ?", clause)
			require.Len(t, values, 1)
			assert.Equal(t, test.value, values[0])
		})
	}
}

// TestBoundEventFilterClauseCombines covers multiple fields AND-ed in a
// stable order with one bound value each.
func TestBoundEventFilterClauseCombines(t *testing.T) {
	clause, values := boundEventFilterClause(&v0.Event{
		Note:                util.Ptr("boom"),
		Type:                util.Ptr("Warning"),
		ReportingController: util.Ptr("kubernetes-workload-controller"),
	})

	assert.Equal(t, 3, strings.Count(clause, " AND "))
	assert.Equal(t, 3, strings.Count(clause, "?"))
	assert.Len(t, values, 3)
	assert.Equal(t, []interface{}{"boom", "Warning", "kubernetes-workload-controller"}, values)
}

// TestBoundEventFilterClauseKeepsValuesOutOfSQL rejects interpolating a
// filter value into the SQL text.
func TestBoundEventFilterClauseKeepsValuesOutOfSQL(t *testing.T) {
	// payload that would be a statement if interpolated
	hostile := "'; DROP TABLE v0_events; --"

	clause, values := boundEventFilterClause(&v0.Event{Note: util.Ptr(hostile)})

	assert.NotContains(t, clause, "DROP TABLE", "the value must not reach the SQL text")
	assert.Equal(t, " AND v0_events.note = ?", clause)
	require.Len(t, values, 1)
	assert.Equal(t, hostile, values[0], "the value travels as a bind parameter")
}

// TestBoundEventFilterClauseSkipsReason covers a Reason-only filter producing
// no clause. Reason query params own that column.
func TestBoundEventFilterClauseSkipsReason(t *testing.T) {
	clause, values := boundEventFilterClause(&v0.Event{Reason: util.Ptr("FailedCreate")})

	assert.Equal(t, "", clause)
	assert.Empty(t, values)
}
