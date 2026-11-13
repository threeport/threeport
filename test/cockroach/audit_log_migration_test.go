package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestAuditLogMigratesAnExistingDatabase covers 000011 on a database built
// before the definitions recorded an audit log, as a deployed control plane's
// is: it adds the column to each kubernetes runtime definition table, and a
// definition already there reads as having none.  On a database 000001 built
// with the field already, it changes nothing, and its down migration removes
// it.
func TestAuditLogMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "audit_log_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	definitions := []any{
		&api_v0.KubernetesRuntimeDefinition{},
		&api_v0.AwsEksKubernetesRuntimeDefinition{},
		&api_v0.GcpGkeKubernetesRuntimeDefinition{},
		&api_v0.OciOkeKubernetesRuntimeDefinition{},
	}
	withColumn := func() int {
		count := 0
		for _, definition := range definitions {
			if db.Migrator().HasColumn(definition, "audit_log") {
				count++
			}
		}
		return count
	}

	// a new install: 000001 builds the field from the models
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.Equal(t, len(definitions), withColumn())
	require.NoError(t, migrations.Up000011(ctx, sqlDb), "000011 finds it there")

	// a database built before the field, holding a definition
	require.NoError(t, migrations.Down000011(ctx, sqlDb))
	assert.Zero(t, withColumn())
	require.NoError(t, migrations.Down000011(ctx, sqlDb), "a down migration with nothing left to remove")
	require.NoError(t, db.Exec(
		"INSERT INTO v0_kubernetes_runtime_definitions (name, infra_provider, created_at, updated_at) VALUES ('before', 'eks', now(), now())",
	).Error)

	require.NoError(t, migrations.Up000011(ctx, sqlDb))
	assert.Equal(t, len(definitions), withColumn(), "on every kubernetes runtime definition table")
	var auditLog *bool
	require.NoError(t, db.Raw(
		"SELECT audit_log FROM v0_kubernetes_runtime_definitions WHERE name = 'before'",
	).Scan(&auditLog).Error)
	require.NotNil(t, auditLog)
	assert.False(t, *auditLog, "a definition from before has no audit log")
}
