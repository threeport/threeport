package v0

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestANewClusterHasBeenAskedForTheVersionItIsCreatedOn covers #797: an EKS
// or GKE instance is created recording, as the version its cluster was asked
// for, the version of its definition, or the default when the definition
// has none, which the controller creates the cluster on.  A version the
// create already carries is kept.  A definition that doesn't exist records
// nothing, and the create fails on its reference instead.  The definition is
// read apart from the create's own clauses, such as a request's filters.
func TestANewClusterHasBeenAskedForTheVersionItIsCreatedOn(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	require.NoError(t, db.AutoMigrate(&AwsEksKubernetesRuntimeInstance{}, &GcpGkeKubernetesRuntimeInstance{}))
	raw := db.Session(&gorm.Session{SkipHooks: true})

	// a filler row, so the definitions' IDs differ from what a filter names
	require.NoError(t, raw.Create(&AwsEksKubernetesRuntimeDefinition{
		Definition: Definition{Name: util.Ptr("filler")}, ZoneCount: util.Ptr(2),
		DefaultNodeGroupInstanceType: util.Ptr("t3.medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
		DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
	}).Error)
	created := 0
	next := func() int { created++; return created }
	eksDefinition := func(version *string) uint {
		definition := &AwsEksKubernetesRuntimeDefinition{
			Definition: Definition{Name: util.Ptr(fmt.Sprintf("eks-%d", next()))}, ZoneCount: util.Ptr(2),
			DefaultNodeGroupInstanceType: util.Ptr("t3.medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
			DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(1)), KubernetesVersion: version,
		}
		require.NoError(t, raw.Create(definition).Error)
		return *definition.ID
	}
	eks := func(t *testing.T, session *gorm.DB, definitionID uint, requested *string) *string {
		t.Helper()
		n := next()
		instance := &AwsEksKubernetesRuntimeInstance{
			Instance:                            Instance{Name: util.Ptr(fmt.Sprintf("eks-%d", n))},
			AwsEksKubernetesRuntimeDefinitionID: util.Ptr(definitionID),
			AwsProviderID:                       util.Ptr(uint(1)),
			KubernetesRuntimeInstanceID:         util.Ptr(uint(n)),
			UpgradeRequestedVersion:             requested,
		}
		require.NoError(t, session.Create(instance).Error)
		var loaded AwsEksKubernetesRuntimeInstance
		require.NoError(t, db.First(&loaded, *instance.ID).Error)
		return loaded.UpgradeRequestedVersion
	}

	t.Run("the definition's version", func(t *testing.T) {
		assert.Equal(t, util.Ptr("1.35"), eks(t, db, eksDefinition(util.Ptr("1.35")), nil))
	})
	t.Run("the default", func(t *testing.T) {
		assert.Equal(t, util.Ptr(DefaultKubernetesVersion), eks(t, db, eksDefinition(nil), nil))
	})
	t.Run("one the create carries", func(t *testing.T) {
		assert.Equal(t, util.Ptr("1.35"), eks(t, db, eksDefinition(util.Ptr("1.36")), util.Ptr("1.35")))
	})
	t.Run("no definition", func(t *testing.T) {
		assert.Nil(t, eks(t, db, 999, nil))
	})
	t.Run("under a request's filters", func(t *testing.T) {
		id := eksDefinition(util.Ptr("1.35"))
		filtered := db.Scopes(func(d *gorm.DB) *gorm.DB { return d.Where("id IN ?", []uint{id + 100}) })
		assert.Equal(t, util.Ptr("1.35"), eks(t, filtered, id, nil))
	})
	t.Run("GKE", func(t *testing.T) {
		definition := &GcpGkeKubernetesRuntimeDefinition{
			Definition: Definition{Name: util.Ptr("gke")}, ZoneCount: util.Ptr(3),
			DefaultNodeGroupInstanceType: util.Ptr("e2-medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
			DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(1)), KubernetesVersion: util.Ptr("1.35"),
		}
		require.NoError(t, raw.Create(definition).Error)
		instance := &GcpGkeKubernetesRuntimeInstance{
			Instance:                            Instance{Name: util.Ptr("gke")},
			GcpGkeKubernetesRuntimeDefinitionID: definition.ID,
			GcpProviderID:                       util.Ptr(uint(1)),
			KubernetesRuntimeInstanceID:         util.Ptr(uint(101)),
		}
		require.NoError(t, db.Create(instance).Error)
		var loaded GcpGkeKubernetesRuntimeInstance
		require.NoError(t, db.First(&loaded, *instance.ID).Error)
		assert.Equal(t, util.Ptr("1.35"), loaded.UpgradeRequestedVersion)

		kept := &GcpGkeKubernetesRuntimeInstance{
			Instance:                            Instance{Name: util.Ptr("gke-kept")},
			GcpGkeKubernetesRuntimeDefinitionID: definition.ID,
			GcpProviderID:                       util.Ptr(uint(1)),
			KubernetesRuntimeInstanceID:         util.Ptr(uint(102)),
			UpgradeRequestedVersion:             util.Ptr("1.36"),
		}
		require.NoError(t, db.Create(kept).Error)
		var loadedKept GcpGkeKubernetesRuntimeInstance
		require.NoError(t, db.First(&loadedKept, *kept.ID).Error)
		assert.Equal(t, util.Ptr("1.36"), loadedKept.UpgradeRequestedVersion)

		missing := &GcpGkeKubernetesRuntimeInstance{
			Instance:                            Instance{Name: util.Ptr("gke-missing")},
			GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(999)),
			GcpProviderID:                       util.Ptr(uint(1)),
			KubernetesRuntimeInstanceID:         util.Ptr(uint(103)),
		}
		require.NoError(t, db.Create(missing).Error)
		var loadedMissing GcpGkeKubernetesRuntimeInstance
		require.NoError(t, db.First(&loadedMissing, *missing.ID).Error)
		assert.Nil(t, loadedMissing.UpgradeRequestedVersion)
	})
}
