package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestGkeNodeSecurityMigratesAnExistingDatabase covers 000015 on a database
// built before GKE definitions and instances recorded secrets encryption,
// boot disks, Secure Boot and intranode visibility, as a deployed control
// plane's is: it adds the six columns to both tables, with the types 000001
// gives them.  On a database 000001 built with the fields already, it changes
// nothing, and its down migration removes them.
func TestGkeNodeSecurityMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "gke_node_security_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	columns := []string{"secrets_encryption_key", "boot_disk_type", "boot_disk_size_gb", "boot_disk_kms_key", "secure_boot", "intranode_visibility"}
	models := map[string]any{
		"v0_gcp_gke_kubernetes_runtime_definitions": &api_v0.GcpGkeKubernetesRuntimeDefinition{},
		"v0_gcp_gke_kubernetes_runtime_instances":   &api_v0.GcpGkeKubernetesRuntimeInstance{},
	}
	types := func() map[string]string {
		var rows []struct{ TableName, ColumnName, DataType string }
		require.NoError(t, db.Raw(
			"SELECT table_name, column_name, data_type FROM information_schema.columns WHERE table_name IN ? AND column_name IN ?",
			[]string{"v0_gcp_gke_kubernetes_runtime_definitions", "v0_gcp_gke_kubernetes_runtime_instances"}, columns,
		).Scan(&rows).Error)
		found := map[string]string{}
		for _, row := range rows {
			found[row.TableName+"."+row.ColumnName] = row.DataType
		}
		return found
	}
	present := func() int {
		count := 0
		for _, model := range models {
			for _, column := range columns {
				if db.Migrator().HasColumn(model, column) {
					count++
				}
			}
		}
		return count
	}

	// a new install: 000001 builds the fields from the models
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.Equal(t, 2*len(columns), present())
	installed := types()
	require.Len(t, installed, 2*len(columns))
	require.NoError(t, migrations.Up000015(ctx, sqlDb), "000015 finds them there")

	// a database built before the fields
	require.NoError(t, migrations.Down000015(ctx, sqlDb))
	assert.Zero(t, present())
	require.NoError(t, migrations.Down000015(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000015(ctx, sqlDb))
	assert.Equal(t, 2*len(columns), present())
	assert.Equal(t, installed, types(), "the types 000001 builds them with")
}
