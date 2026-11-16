package cockroach

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// networkCidrIndex is the partial unique index on a kubernetes runtime
// instance's network range, under the name GORM gives it.
const networkCidrIndex = "idx_v0_kubernetes_runtime_instances_network_cidr"

// TestNetworkRangesMigrateAnExistingDatabase covers 000002 on a database
// built before the network range fields, as a deployed control plane's is: it
// adds both columns and the partial unique index, which then refuses a range
// two runtimes not deleted share.  On a database 000001 built with the fields
// already, it changes nothing, and its down migration removes them.
func TestNetworkRangesMigrateAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "network_cidr_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	runtimeHas := func(column string) bool {
		return db.Migrator().HasColumn(&api_v0.KubernetesRuntimeInstance{}, column)
	}
	controlPlaneHas := func(column string) bool {
		return db.Migrator().HasColumn(&api_v0.ControlPlaneInstance{}, column)
	}
	indexDefinition := func() string {
		var definition string
		require.NoError(t, db.Raw(
			"SELECT COALESCE(MAX(indexdef), '') FROM pg_indexes WHERE tablename = ? AND indexname = ?",
			"v0_kubernetes_runtime_instances", networkCidrIndex,
		).Scan(&definition).Error)
		return definition
	}

	// a new install: 000001 builds the fields from the models, under the
	// names 000002 adds them by
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.True(t, runtimeHas("network_cidr"))
	require.True(t, controlPlaneHas("network_cidr_pool"))
	require.Contains(t, indexDefinition(), "WHERE", "GORM names the partial index as 000002 does")
	require.NoError(t, migrations.Up000002(ctx, sqlDb), "000002 finds them there")

	// a database built before the fields
	require.NoError(t, migrations.Down000002(ctx, sqlDb))
	assert.False(t, runtimeHas("network_cidr"))
	assert.False(t, controlPlaneHas("network_cidr_pool"))
	assert.Empty(t, indexDefinition())
	require.NoError(t, migrations.Down000002(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000002(ctx, sqlDb))
	assert.True(t, runtimeHas("network_cidr"))
	assert.True(t, controlPlaneHas("network_cidr_pool"))
	assert.Contains(t, indexDefinition(), "UNIQUE INDEX")
	assert.Contains(t, indexDefinition(), "WHERE (deleted_at IS NULL)")

	// the index refuses a range two runtimes not deleted share, as the
	// hooks' check does above it
	definition := &api_v0.KubernetesRuntimeDefinition{
		Definition:    api_v0.Definition{Name: util.Ptr("network-cidr-migration-definition")},
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
	}
	withoutHooks := db.Session(&gorm.Session{SkipHooks: true})
	require.NoError(t, withoutHooks.Create(definition).Error)
	create := func(name string) (*api_v0.KubernetesRuntimeInstance, error) {
		instance := &api_v0.KubernetesRuntimeInstance{
			Instance:                      api_v0.Instance{Name: util.Ptr(name)},
			Location:                      util.Ptr("Local"),
			KubernetesRuntimeDefinitionID: definition.ID,
			NetworkCidr:                   util.Ptr("10.16.0.0/16"),
		}
		return instance, withoutHooks.Create(instance).Error
	}
	first, err := create("network-cidr-migration-first")
	require.NoError(t, err)
	_, err = create("network-cidr-migration-second")
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "got %v", err)
	assert.Equal(t, "23505", pgErr.Code)
	require.NoError(t, withoutHooks.Delete(first).Error)
	_, err = create("network-cidr-migration-after-delete")
	assert.NoError(t, err, "a deleted runtime's range is free again")
}
