package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestGkeUpgradeWindowsMigrateAnExistingDatabase covers 000005 on a database
// built before GKE definitions recorded a release channel and maintenance
// window, as a deployed control plane's is: it adds the four columns.  On a
// database 000001 built with the fields already, it changes nothing, and its
// down migration removes them.
func TestGkeUpgradeWindowsMigrateAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "gke_upgrade_windows_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	columns := []string{"release_channel", "maintenance_window_days", "maintenance_window_start", "maintenance_window_hours"}
	types := func() map[string]string {
		var rows []struct{ ColumnName, DataType string }
		require.NoError(t, db.Raw(
			"SELECT column_name, data_type FROM information_schema.columns WHERE table_name = ? AND column_name IN ?",
			"v0_gcp_gke_kubernetes_runtime_definitions", columns,
		).Scan(&rows).Error)
		found := map[string]string{}
		for _, row := range rows {
			found[row.ColumnName] = row.DataType
		}
		return found
	}
	present := func() int {
		count := 0
		for _, column := range columns {
			if db.Migrator().HasColumn(&api_v0.GcpGkeKubernetesRuntimeDefinition{}, column) {
				count++
			}
		}
		return count
	}

	// a new install: 000001 builds the fields from the model
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.Equal(t, len(columns), present())
	installed := types()
	require.NoError(t, migrations.Up000005(ctx, sqlDb), "000005 finds them there")

	// a database built before the fields
	require.NoError(t, migrations.Down000005(ctx, sqlDb))
	assert.Zero(t, present())
	require.NoError(t, migrations.Down000005(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000005(ctx, sqlDb))
	assert.Equal(t, len(columns), present())
	assert.Equal(t, installed, types(), "the types 000001 builds them with")
}
