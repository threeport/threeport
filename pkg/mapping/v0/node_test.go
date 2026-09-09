package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidMachineSize covers that a known node size is accepted and an unknown
// one is rejected.
func TestValidMachineSize(t *testing.T) {
	// accepts a node size present in the machine type map
	assert.True(t, ValidMachineSize("Small"))
	// rejects a node size absent from the machine type map
	assert.False(t, ValidMachineSize("NotARealSize"))
	// accepts a size only one profile carries, so the check is known to be
	// profile-agnostic rather than accidentally strict
	assert.True(t, ValidMachineSize("2XSmall"))
}
