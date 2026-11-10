package cockroach

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// migrateRuntimeTables migrates the runtime tables once: a second AutoMigrate
// of a table with a unique column fails on CockroachDB, which has no named
// constraint for it to find.
var migrateRuntimeTables = sync.OnceValue(func() error {
	return testDb.AutoMigrate(&api_v0.KubernetesRuntimeDefinition{}, &api_v0.KubernetesRuntimeInstance{})
})

// setupRuntimeTables readies the runtime tables and the key their hooks
// encrypt with.
func setupRuntimeTables(t *testing.T) {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)
	require.NoError(t, migrateRuntimeTables())
}

// TestNetworkRangesOnCockroach covers a kubernetes runtime instance's
// NetworkCidr on CockroachDB: the hooks refuse a range that overlaps another
// runtime's, the partial unique index refuses a duplicate that gets past them,
// and a deleted runtime frees its range.
func TestNetworkRangesOnCockroach(t *testing.T) {
	setupRuntimeTables(t)

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

// TestTwoAllocationsToOneRuntimeOnCockroach covers two updates that give the
// same runtime a range at once.  Each loaded the runtime without one, so the
// second must read the row again, after the first commits, to see that its
// range is set.
func TestTwoAllocationsToOneRuntimeOnCockroach(t *testing.T) {
	setupRuntimeTables(t)

	definition := &api_v0.KubernetesRuntimeDefinition{
		Definition:    api_v0.Definition{Name: util.Ptr("network-cidr-race-definition")},
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
	}
	require.NoError(t, testDb.Create(definition).Error)
	runtime := &api_v0.KubernetesRuntimeInstance{
		Instance:                      api_v0.Instance{Name: util.Ptr("network-cidr-race")},
		Location:                      util.Ptr("Local"),
		KubernetesRuntimeDefinitionID: definition.ID,
	}
	require.NoError(t, testDb.Create(runtime).Error)
	load := func() *api_v0.KubernetesRuntimeInstance {
		var loaded api_v0.KubernetesRuntimeInstance
		require.NoError(t, testDb.First(&loaded, *runtime.ID).Error)
		return &loaded
	}
	firstLoaded, secondLoaded := load(), load()

	// the first allocation holds its transaction open while the second starts
	first := testDb.Begin()
	require.NoError(t, first.Model(firstLoaded).Updates(&api_v0.KubernetesRuntimeInstance{
		NetworkCidr: util.Ptr("10.201.0.0/16"),
	}).Error)
	second := make(chan error, 1)
	go func() {
		second <- testDb.Model(secondLoaded).Updates(&api_v0.KubernetesRuntimeInstance{
			NetworkCidr: util.Ptr("10.202.0.0/16"),
		}).Error
	}()
	select {
	case err := <-second:
		t.Fatalf("the second allocation finished while the first held the row: %v", err)
	case <-time.After(2 * time.Second):
	}
	require.NoError(t, first.Commit().Error)

	select {
	case err := <-second:
		assert.ErrorContains(t, err, api_v0.ErrMsgNetworkCidrImmutable)
	case <-time.After(30 * time.Second):
		t.Fatal("the second allocation did not finish after the first committed")
	}
	assert.Equal(t, "10.201.0.0/16", *load().NetworkCidr)
}
