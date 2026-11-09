package cockroach

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestNetworkRangesOnCockroach covers a kubernetes runtime instance's
// NetworkCidr on CockroachDB: the hooks refuse a range that overlaps another
// runtime's, the partial unique index refuses a duplicate that gets past them,
// and a deleted runtime frees its range.
func TestNetworkRangesOnCockroach(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)
	require.NoError(t, testDb.AutoMigrate(&api_v0.KubernetesRuntimeDefinition{}, &api_v0.KubernetesRuntimeInstance{}))

	definition := &api_v0.KubernetesRuntimeDefinition{
		Definition:    api_v0.Definition{Name: util.Ptr("network-cidr-definition")},
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
	}
	require.NoError(t, testDb.Create(definition).Error)
	create := func(db *gorm.DB, name, networkCidr string) (*api_v0.KubernetesRuntimeInstance, error) {
		instance := &api_v0.KubernetesRuntimeInstance{
			Instance:                      api_v0.Instance{Name: util.Ptr(name)},
			Location:                      util.Ptr("Local"),
			KubernetesRuntimeDefinitionID: definition.ID,
			NetworkCidr:                   util.Ptr(networkCidr),
		}
		return instance, db.Create(instance).Error
	}

	first, err := create(testDb, "network-cidr-first", "10.200.0.0/16")
	require.NoError(t, err)

	_, err = create(testDb, "network-cidr-holding-it", "10.200.0.0/15")
	assert.ErrorContains(t, err, "overlaps 10.200.0.0/16, the network range of kubernetes runtime instance network-cidr-first")

	_, err = create(testDb.Session(&gorm.Session{SkipHooks: true}), "network-cidr-duplicate", "10.200.0.0/16")
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "got %v", err)
	assert.Equal(t, "23505", pgErr.Code)

	require.NoError(t, testDb.Session(&gorm.Session{SkipHooks: true}).Delete(first).Error)
	_, err = create(testDb, "network-cidr-after-delete", "10.200.0.0/16")
	assert.NoError(t, err, "a deleted runtime's range is free again")
}
