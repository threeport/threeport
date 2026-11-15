package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestClusterUpgradesMigratesAnExistingDatabase covers 000006 on a database
// built before the EKS and GKE instances recorded their version and
// upgrades, as a deployed control plane's is: it adds each column to both
// instance tables, and an instance already there isn't upgrading.  On a
// database 000001 built with the fields already, it changes nothing, and its
// down migration removes them.
func TestClusterUpgradesMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "cluster_upgrades_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	instances := []any{&api_v0.AwsEksKubernetesRuntimeInstance{}, &api_v0.GcpGkeKubernetesRuntimeInstance{}}
	columns := []string{
		"kubernetes_version", "upgrade_requested_version", "upgrade_acknowledged",
		"upgrading", "upgrade_failed", "upgrade_refused", "upgrade_status",
	}
	withColumns := func() int {
		count := 0
		for _, instance := range instances {
			for _, column := range columns {
				if db.Migrator().HasColumn(instance, column) {
					count++
				}
			}
		}
		return count
	}
	all := len(instances) * len(columns)

	// a new install: 000001 builds the fields from the models
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.Equal(t, all, withColumns())
	require.NoError(t, migrations.Up000006(ctx, sqlDb), "000006 finds them there")

	// a database built before the fields
	require.NoError(t, migrations.Down000006(ctx, sqlDb))
	assert.Zero(t, withColumns())
	require.NoError(t, migrations.Down000006(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000006(ctx, sqlDb))
	assert.Equal(t, all, withColumns(), "on both instance tables")
	var upgrading *bool
	require.NoError(t, db.Raw(
		"SELECT column_default IS NOT NULL FROM information_schema.columns WHERE table_name = 'v0_gcp_gke_kubernetes_runtime_instances' AND column_name = 'upgrading'",
	).Scan(&upgrading).Error)
	require.NotNil(t, upgrading)
	assert.True(t, *upgrading, "an instance from before reads as not upgrading")
}
