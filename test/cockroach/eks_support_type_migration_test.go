package cockroach

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/cmd/database-migrator/migrations"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestEksSupportTypeMigratesAnExistingDatabase covers 000017 on a database
// built before EKS definitions recorded a support type, as a deployed
// control plane's is: it adds the column, with no value for the definitions
// already there, which read as the default.  On a database 000001 built
// with the field already, it changes nothing, and its down migration
// removes it.
func TestEksSupportTypeMigratesAnExistingDatabase(t *testing.T) {
	db := freshDatabase(t, "eks_support_type_migration")
	//nolint:staticcheck // SA1029: the key has to match what the migration reads
	ctx := context.WithValue(context.Background(), "gormdb", db)
	sqlDb, err := db.DB()
	require.NoError(t, err)
	hasColumn := func() bool {
		return db.Migrator().HasColumn(&api_v0.AwsEksKubernetesRuntimeDefinition{}, "support_type")
	}
	columnType := func() string {
		var dataType string
		require.NoError(t, db.Raw(
			"SELECT data_type FROM information_schema.columns WHERE table_name = ? AND column_name = ?",
			"v0_aws_eks_kubernetes_runtime_definitions", "support_type",
		).Scan(&dataType).Error)
		return dataType
	}

	// a new install: 000001 builds the field from the model
	require.NoError(t, migrations.Up000001(ctx, nil))
	require.True(t, hasColumn())
	installed := columnType()
	require.NoError(t, migrations.Up000017(ctx, sqlDb), "000017 finds it there")

	// a database built before the field, with a definition in it
	runtime := createEksRuntime(t, db, "existing", nil)
	require.NoError(t, migrations.Down000017(ctx, sqlDb))
	assert.False(t, hasColumn())
	require.NoError(t, migrations.Down000017(ctx, sqlDb), "a down migration with nothing left to remove")

	require.NoError(t, migrations.Up000017(ctx, sqlDb))
	assert.True(t, hasColumn())
	assert.Equal(t, installed, columnType(), "the type 000001 builds it with")
	require.NoError(t, migrations.Up000017(ctx, sqlDb), "run again")

	var existing api_v0.AwsEksKubernetesRuntimeDefinition
	require.NoError(t, db.First(&existing, *runtime.eksDefinition.ID).Error)
	assert.Nil(t, existing.SupportType)
	assert.Equal(t, api_v0.DefaultEksSupportType, api_v0.EksSupportTypeOrDefault(existing.SupportType),
		"which reads as the default")
}
