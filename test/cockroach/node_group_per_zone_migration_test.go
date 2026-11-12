package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestNodeGroupPerZoneMigratesAnExistingDatabase covers 000006 on a database
// built before EKS definitions recorded NodeGroupPerZone, as a deployed
// control plane's is: it adds the column, false for the clusters already
// there, with the type and default 000001 builds it with.  On a database
// 000001 built with the field already, it changes nothing, and its down
// migration removes it.
func TestNodeGroupPerZoneMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "node_group_per_zone_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	hasColumn := func() bool {
		return db.Migrator().HasColumn(&api_v0.AwsEksKubernetesRuntimeDefinition{}, "node_group_per_zone")
	}
	column := func() string {
		var description string
		require.NoError(t, db.Raw(
			"SELECT data_type || ' ' || COALESCE(column_default, '') FROM information_schema.columns WHERE table_name = ? AND column_name = ?",
			"v0_aws_eks_kubernetes_runtime_definitions", "node_group_per_zone",
		).Scan(&description).Error)
		return description
	}

	// a new install: 000001 builds the field from the model
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.True(t, hasColumn())
	installed := column()
	require.NoError(t, migrations.Up000006(ctx, sqlDb), "000006 finds it there")

	// a database built before the field
	require.NoError(t, migrations.Down000006(ctx, sqlDb))
	assert.False(t, hasColumn())
	require.NoError(t, migrations.Down000006(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000006(ctx, sqlDb))
	assert.True(t, hasColumn())
	assert.Equal(t, installed, column(), "the type and default 000001 builds it with")
	assert.Contains(t, column(), "false", "existing clusters run one node group spanning every zone")
}
