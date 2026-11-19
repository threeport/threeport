package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStorageSizeFlagsAreEachChecked covers the --message-broker-storage-size
// and --database-storage-size flags: each refused, by its own name, when it
// isn't a positive size, and either left empty for the tier's.
func TestStorageSizeFlagsAreEachChecked(t *testing.T) {
	assert.NoError(t, ValidateStorageSizeFlags("", ""), "the tier's sizes")
	assert.NoError(t, ValidateStorageSizeFlags("20Gi", "250Gi"))

	assert.ErrorContains(t, ValidateStorageSizeFlags("lots", "250Gi"), `--message-broker-storage-size "lots" is not a size such as 250Gi`)
	assert.ErrorContains(t, ValidateStorageSizeFlags("20Gi", "0"), "--database-storage-size 0 is not a size greater than zero")
	assert.ErrorContains(t, ValidateStorageSizeFlags("", "-5Gi"), "--database-storage-size -5Gi")
}
