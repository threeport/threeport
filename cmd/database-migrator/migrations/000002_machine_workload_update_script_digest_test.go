package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// A migration that compiles is not a migration that runs. This exercises the
// column add and drop against a real database, so a field name that does not
// resolve fails here rather than against a deployed control plane.
func TestMachineWorkloadUpdateScriptDigestColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// the table as it stood before this migration
	require.NoError(t, db.Migrator().CreateTable(&v0.MachineWorkloadInstance{}))
	require.NoError(t, db.Migrator().DropColumn(&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest))
	require.False(t, db.Migrator().HasColumn(&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest))

	require.NoError(t, addUpdateScriptDigest(db))
	assert.True(
		t, db.Migrator().HasColumn(&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest),
		"the migration did not add the column",
	)

	// running it again is not an error: a partial run has to be able to finish
	require.NoError(t, addUpdateScriptDigest(db))

	require.NoError(t, dropUpdateScriptDigest(db))
	assert.False(t, db.Migrator().HasColumn(&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest))

	// and neither is dropping what is not there
	require.NoError(t, dropUpdateScriptDigest(db))
}
