package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestReservedNatAddressMigratesAnExistingDatabase covers 000003 on a
// database built before ReservedNatAddress, as a deployed control plane's
// is: it adds the column with false for the clusters already there, which
// were created without a reserved address.  On a database 000001 built with
// the field already, it changes nothing, and its down migration removes it.
func TestReservedNatAddressMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "reserved_nat_address_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	hasColumn := func() bool {
		return db.Migrator().HasColumn(&api_v0.GcpGkeKubernetesRuntimeInstance{}, "reserved_nat_address")
	}
	columnDefault := func() string {
		var value string
		require.NoError(t, db.Raw(
			"SELECT COALESCE(column_default, '') FROM information_schema.columns WHERE table_name = ? AND column_name = ?",
			"v0_gcp_gke_kubernetes_runtime_instances", "reserved_nat_address",
		).Scan(&value).Error)
		return value
	}

	// a new install: 000001 builds the field from the model
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.True(t, hasColumn())
	require.NoError(t, migrations.Up000003(ctx, sqlDb), "000003 finds it there")

	// a database built before the field
	require.NoError(t, migrations.Down000003(ctx, sqlDb))
	assert.False(t, hasColumn())
	require.NoError(t, migrations.Down000003(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000003(ctx, sqlDb))
	assert.True(t, hasColumn())
	assert.Equal(t, "false", columnDefault(), "existing clusters were created without a reserved address")
}
