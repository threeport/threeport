package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidLocation covers that a known threeport location is accepted and an
// unknown one is rejected.
func TestValidLocation(t *testing.T) {
	// accepts a location present in the region map
	assert.True(t, ValidLocation("Local"))
	// rejects a location absent from the region map
	assert.False(t, ValidLocation("NotARealLocation"))
}
