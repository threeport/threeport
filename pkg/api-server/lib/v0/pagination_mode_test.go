package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidPaginationMode verifies the two supported modes are accepted and
// everything else is rejected. The value arrives from the REST API server's
// -pagination-mode flag, so an unrecognized spelling has to fail at startup
// rather than reach the query layer.
func TestValidPaginationMode(t *testing.T) {
	tests := []struct {
		name  string
		mode  string
		valid bool
	}{
		{"as of system time", string(PaginationModeAsOfSystemTime), true},
		{"materialized view", string(PaginationModeMaterializedView), true},

		{"empty", "", false},
		{"unknown mode", "snapshot", false},
		{"underscores instead of hyphens", "as_of_system_time", false},
		{"spaces instead of hyphens", "as of system time", false},
		{"uppercase", "AS-OF-SYSTEM-TIME", false},
		{"leading space", " as-of-system-time", false},
		{"trailing space", "as-of-system-time ", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.valid, ValidPaginationMode(test.mode))
		})
	}
}

// TestPaginationModeConstants pins the wire spellings. They are written by
// hand into installer flags and deployment manifests, so a rename that changed
// them silently would leave a running control plane rejecting its own flag.
func TestPaginationModeConstants(t *testing.T) {
	assert.Equal(t, "as-of-system-time", string(PaginationModeAsOfSystemTime))
	assert.Equal(t, "materialized-view", string(PaginationModeMaterializedView))
}
