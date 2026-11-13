package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
)

// TestNodeMaximumDefaultMigratesAnExistingDatabase covers 000010 on a
// database built when NodeMaximum defaulted to 250, as a deployed control
// plane's is: a definition created without one afterwards gets 60, while a
// definition already there keeps the maximum it records.  It changes nothing
// on a database 000001 built with the new default, and its down migration
// restores 250.
func TestNodeMaximumDefaultMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "node_maximum_default_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	insert := func(name string) {
		require.NoError(t, db.Exec(
			"INSERT INTO v0_kubernetes_runtime_definitions (name, infra_provider, created_at, updated_at) VALUES (?, 'gke', now(), now())",
			name,
		).Error)
	}
	maximum := func(name string) int {
		var nodeMaximum int
		require.NoError(t, db.Raw("SELECT node_maximum FROM v0_kubernetes_runtime_definitions WHERE name = ?", name).Scan(&nodeMaximum).Error)
		return nodeMaximum
	}

	// a new install: 000001 builds the column with the new default
	require.NoError(t, migrations.Up000001(ctx, nil))
	insert("new-install")
	assert.Equal(t, 60, maximum("new-install"))
	require.NoError(t, migrations.Up000010(ctx, sqlDb), "000010 on a default already set")

	// a database from before, holding a definition
	require.NoError(t, migrations.Down000010(ctx, sqlDb))
	insert("before")
	assert.Equal(t, 250, maximum("before"))

	require.NoError(t, migrations.Up000010(ctx, sqlDb))
	insert("after")
	assert.Equal(t, 250, maximum("before"), "a definition already there keeps its maximum")
	assert.Equal(t, 60, maximum("after"))
}
