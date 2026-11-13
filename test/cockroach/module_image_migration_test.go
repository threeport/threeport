package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestModuleImageColumnsMigrateAnExistingDatabase covers 000007 on a database
// built before a module recorded its image, and on one 000001 built with the
// columns already there.
func TestModuleImageColumnsMigrateAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "module_image_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	columns := []string{"image_namespace", "image_tag"}
	present := func() int {
		count := 0
		for _, column := range columns {
			if db.Migrator().HasColumn(&api_v0.ModuleApi{}, column) {
				count++
			}
		}
		return count
	}

	// a new install: 000001 builds the fields from the model
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.Equal(t, len(columns), present())
	require.NoError(t, migrations.Up000007(ctx, sqlDb), "000007 finds them there")

	// a database built before the fields
	require.NoError(t, migrations.Down000007(ctx, sqlDb))
	assert.Zero(t, present())
	require.NoError(t, migrations.Down000007(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000007(ctx, sqlDb))
	assert.Equal(t, len(columns), present())
}
