package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestReservedNetworkCidrsMigrateAnExistingDatabase covers 000007 on a
// database built before control planes reserved their deleted children's
// pools, as a deployed control plane's is: it adds the column with the type
// 000001 builds it with.  On a database 000001 built with the field already,
// it changes nothing, and its down migration removes it.
func TestReservedNetworkCidrsMigrateAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "reserved_network_cidrs_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	hasColumn := func() bool {
		return db.Migrator().HasColumn(&api_v0.ControlPlaneInstance{}, "reserved_network_cidrs")
	}
	columnType := func() string {
		var dataType string
		require.NoError(t, db.Raw(
			"SELECT data_type FROM information_schema.columns WHERE table_name = ? AND column_name = ?",
			"v0_control_plane_instances", "reserved_network_cidrs",
		).Scan(&dataType).Error)
		return dataType
	}

	// a new install: 000001 builds the field from the model
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.True(t, hasColumn())
	installed := columnType()
	require.NoError(t, migrations.Up000007(ctx, sqlDb), "000007 finds it there")

	// a database built before the field
	require.NoError(t, migrations.Down000007(ctx, sqlDb))
	assert.False(t, hasColumn())
	require.NoError(t, migrations.Down000007(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000007(ctx, sqlDb))
	assert.True(t, hasColumn())
	assert.Equal(t, installed, columnType(), "the type 000001 builds it with")
}
