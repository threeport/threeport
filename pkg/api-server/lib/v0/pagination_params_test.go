package v0

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPaginationContext wraps a GET at target in the CustomContext the
// generated handlers receive, so GetPaginationParams is exercised through the
// same type production code calls it on.
func newPaginationContext(target string) *CustomContext {
	c, _ := newBindContext(http.MethodGet, target, nil)
	return &CustomContext{Context: c}
}

// TestGetPaginationParamsDefaults verifies a request carrying no pagination
// query params comes back with the default limit and an empty query id and
// cursor, which is what a client's first unpaginated list request looks like.
func TestGetPaginationParamsDefaults(t *testing.T) {
	params, err := newPaginationContext("/v0/kubernetes-workload-definitions").GetPaginationParams()

	require.NoError(t, err)
	assert.Equal(t, int64(DefaultPaginationLimitValue), params.Limit)
	assert.Equal(t, "", params.QueryId)
	assert.Equal(t, uint(0), params.Cursor)
}

// TestGetPaginationParamsAccepted verifies the values a well-formed
// continuation request carries are parsed onto the params struct.
func TestGetPaginationParamsAccepted(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		queryId string
		cursor  uint
		limit   int64
	}{
		{"limit only", "/?limit=25", "", 0, 25},
		{"limit at the maximum", fmt.Sprintf("/?limit=%d", MaxPaginationLimitValue), "", 0, MaxPaginationLimitValue},
		{"limit of one", "/?limit=1", "", 0, 1},
		{"cursor only", "/?cursor=42", "", 42, DefaultPaginationLimitValue},
		{"query id only", "/?queryid=1712345678901234567.0000000001", "1712345678901234567.0000000001", 0, DefaultPaginationLimitValue},
		{
			"full continuation",
			"/?queryid=1712345678901234567.0000000001&cursor=42&limit=10",
			"1712345678901234567.0000000001", 42, 10,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, err := newPaginationContext(test.target).GetPaginationParams()

			require.NoError(t, err)
			assert.Equal(t, test.queryId, params.QueryId)
			assert.Equal(t, test.cursor, params.Cursor)
			assert.Equal(t, test.limit, params.Limit)
		})
	}
}

// TestGetPaginationParamsRejected verifies each malformed or out-of-range
// value is rejected with an error naming what was wrong. The handlers map this
// error to a 400, so anything that slips through here reaches the SQL layer
// and surfaces as a 500 instead.
func TestGetPaginationParamsRejected(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		errContains string
	}{
		{"cursor is not a number", "/?cursor=abc", "invalid cursor value"},
		{"cursor is a decimal", "/?cursor=1.5", "invalid cursor value"},
		{"limit is not a number", "/?limit=abc", "invalid limit value"},
		{"limit is a decimal", "/?limit=10.5", "invalid limit value"},
		{"limit is zero", "/?limit=0", "must be positive"},
		{"limit is negative", "/?limit=-1", "must be positive"},
		{
			"limit is over the maximum",
			fmt.Sprintf("/?limit=%d", MaxPaginationLimitValue+1),
			"too large",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newPaginationContext(test.target).GetPaginationParams()

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.errContains)
		})
	}
}

// TestGetPaginationParamsZeroLimitRejected pins the non-positive limit
// rejection on its own. A zero limit parses cleanly and reads as harmless, but
// it builds a LIMIT 0 page: materialized-view mode creates a view for the
// query, returns no rows, and the empty page never triggers the tail cleanup,
// so the view is left for the TTL sweeper.
func TestGetPaginationParamsZeroLimitRejected(t *testing.T) {
	_, err := newPaginationContext("/?limit=0").GetPaginationParams()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit value must be positive")
	assert.Contains(t, err.Error(), "0", "the rejected value belongs in the message")
}
