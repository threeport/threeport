package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAnUnrelatedDefinitionUpdateLeavesNewClustersAlone covers #797: an
// instance created on its definition's version has been asked for it, so an
// update of the definition that leaves the version alone, such as raising a
// node group's maximum, leaves the instance reconciled and notifies nothing.
// Changing the version still asks it, and pinning the default it was
// created on doesn't.
func TestAnUnrelatedDefinitionUpdateLeavesNewClustersAlone(t *testing.T) {
	t.Run("EKS", func(t *testing.T) {
		h, stream := newUpgradeHandler(t)
		definition := &api.AwsEksKubernetesRuntimeDefinition{
			Common: api.Common{ID: util.Ptr(uint(3))}, Definition: api.Definition{Name: util.Ptr("eks")},
			ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
			DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
		}
		require.NoError(t, h.DB.Session(&gorm.Session{SkipHooks: true}).Create(definition).Error)
		instance := &api.AwsEksKubernetesRuntimeInstance{
			Instance:                            api.Instance{Name: util.Ptr("eks")},
			Reconciliation:                      api.Reconciliation{Reconciled: util.Ptr(true)},
			AwsEksKubernetesRuntimeDefinitionID: definition.ID,
			AwsProviderID:                       util.Ptr(uint(1)),
			KubernetesRuntimeInstanceID:         util.Ptr(uint(11)),
		}
		require.NoError(t, h.DB.Create(instance).Error, "created as the API creates it")
		load := func() api.AwsEksKubernetesRuntimeInstance {
			var loaded api.AwsEksKubernetesRuntimeInstance
			require.NoError(t, h.DB.First(&loaded, *instance.ID).Error)
			return loaded
		}
		assert.Equal(t, util.Ptr(api.DefaultKubernetesVersion), load().UpgradeRequestedVersion, "the default it is created on")

		require.NoError(t, h.DB.Model(definition).Updates(&api.AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(5)}).Error)
		runDefinitionUpdate(t, h.RequestAwsEksUpgrades, "3", http.StatusOK)
		assert.True(t, *load().Reconciled)
		assert.Empty(t, stream.published)

		require.NoError(t, h.DB.Model(definition).Updates(&api.AwsEksKubernetesRuntimeDefinition{KubernetesVersion: util.Ptr(api.DefaultKubernetesVersion)}).Error)
		runDefinitionUpdate(t, h.RequestAwsEksUpgrades, "3", http.StatusOK)
		assert.True(t, *load().Reconciled, "pinning the default it was created on is no new version")
		assert.Empty(t, stream.published)
	})

	t.Run("GKE", func(t *testing.T) {
		h, stream := newUpgradeHandler(t)
		definition := &api.GcpGkeKubernetesRuntimeDefinition{
			Common: api.Common{ID: util.Ptr(uint(3))}, Definition: api.Definition{Name: util.Ptr("gke")},
			ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"), DefaultNodeGroupInitialSize: util.Ptr(1),
			DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(1)), KubernetesVersion: util.Ptr("1.35"),
		}
		require.NoError(t, h.DB.Session(&gorm.Session{SkipHooks: true}).Create(definition).Error)
		instance := &api.GcpGkeKubernetesRuntimeInstance{
			Instance:                            api.Instance{Name: util.Ptr("gke")},
			Reconciliation:                      api.Reconciliation{Reconciled: util.Ptr(true)},
			GcpGkeKubernetesRuntimeDefinitionID: definition.ID,
			GcpProviderID:                       util.Ptr(uint(1)),
			KubernetesRuntimeInstanceID:         util.Ptr(uint(11)),
		}
		require.NoError(t, h.DB.Create(instance).Error, "created as the API creates it")
		load := func() api.GcpGkeKubernetesRuntimeInstance {
			var loaded api.GcpGkeKubernetesRuntimeInstance
			require.NoError(t, h.DB.First(&loaded, *instance.ID).Error)
			return loaded
		}
		assert.Equal(t, util.Ptr("1.35"), load().UpgradeRequestedVersion, "the definition's version it is created on")

		require.NoError(t, h.DB.Model(definition).Updates(&api.GcpGkeKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(5)}).Error)
		runDefinitionUpdate(t, h.RequestGcpGkeUpgrades, "3", http.StatusOK)
		assert.True(t, *load().Reconciled)
		assert.Empty(t, stream.published)

		require.NoError(t, h.DB.Model(definition).Updates(&api.GcpGkeKubernetesRuntimeDefinition{KubernetesVersion: util.Ptr("1.36")}).Error)
		runDefinitionUpdate(t, h.RequestGcpGkeUpgrades, "3", http.StatusOK)
		assert.False(t, *load().Reconciled, "a new version is asked for")
		assert.Len(t, stream.published, 1)
	})
}
