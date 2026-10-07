package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// setupKubernetesWorkloadInstanceDB returns an in-memory sqlite DB with the
// kubernetes workload instance table migrated plus the
// AttachedObjectReference table the relationship-tagged-field dispatcher
// consults on every create/update.
func setupKubernetesWorkloadInstanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&KubernetesWorkloadInstance{},
		&AttachedObjectReference{},
	))
	return db
}

// newTestKubernetesWorkloadInstance returns a KubernetesWorkloadInstance
// with the required fields set, ready to Create against
// setupKubernetesWorkloadInstanceDB's schema.
func newTestKubernetesWorkloadInstance() *KubernetesWorkloadInstance {
	return &KubernetesWorkloadInstance{
		Instance:                       Instance{Name: util.Ptr("test-instance")},
		KubernetesRuntimeInstanceID:    util.Ptr(uint(1)),
		KubernetesWorkloadDefinitionID: util.Ptr(uint(1)),
	}
}

// TestValidateKustomizeOverlaySize_Create covers the create path: an
// oversized KustomizeOverlay must be rejected, and one at or under the
// limit must be accepted.
func TestValidateKustomizeOverlaySize_Create(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		expectErr bool
	}{
		{"nil overlay", -1, false},
		{"well under limit", 10, false},
		{"exactly at limit", MaxKustomizeOverlayBytes, false},
		{"one byte over limit", MaxKustomizeOverlayBytes + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupKubernetesWorkloadInstanceDB(t)
			instance := newTestKubernetesWorkloadInstance()
			if tt.size >= 0 {
				overlay := strings.Repeat("a", tt.size)
				instance.KustomizeOverlay = &overlay
			}

			err := db.Create(instance).Error
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "exceeds the")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateKustomizeOverlayImmutable_Update covers the two GORM call
// shapes a PATCH/PUT can take (see pkg/api/lib/v0/update_helpers.go):
// Updates (partial, PATCH) and Save (full replace, PUT). KustomizeOverlay
// must be rejected as immutable under both shapes whenever it actually
// changes - the reconciler only renders it once, at instance creation
// (v0KubernetesWorkloadInstanceCreated), and never re-renders on update, so
// an accepted change would otherwise persist in the database while never
// reaching the cluster.
func TestValidateKustomizeOverlayImmutable_Update(t *testing.T) {
	original := strings.Repeat("a", 10)
	different := strings.Repeat("b", 10)

	t.Run("PATCH (Updates) rejects changing an existing overlay", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		instance.KustomizeOverlay = &original
		require.NoError(t, db.Create(instance).Error)

		err := db.Model(instance).
			Updates(&KubernetesWorkloadInstance{KustomizeOverlay: &different}).Error
		require.Error(t, err)
		require.Contains(t, err.Error(), "immutable")
	})

	t.Run("PATCH (Updates) rejects setting a previously-nil overlay", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		require.NoError(t, db.Create(instance).Error)

		err := db.Model(instance).
			Updates(&KubernetesWorkloadInstance{KustomizeOverlay: &original}).Error
		require.Error(t, err)
		require.Contains(t, err.Error(), "immutable")
	})

	t.Run("PATCH (Updates) leaving KustomizeOverlay unset does not trip the immutability check", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		instance.KustomizeOverlay = &original
		require.NoError(t, db.Create(instance).Error)

		newName := "renamed"
		err := db.Model(instance).
			Updates(&KubernetesWorkloadInstance{Instance: Instance{Name: &newName}}).Error
		require.NoError(t, err)
	})

	t.Run("PUT (Save) rejects changing an existing overlay", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		instance.KustomizeOverlay = &original
		require.NoError(t, db.Create(instance).Error)

		instance.KustomizeOverlay = &different
		err := db.Save(instance).Error
		require.Error(t, err)
		require.Contains(t, err.Error(), "immutable")
	})

	t.Run("PUT (Save) accepts resending the same overlay value unchanged", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		instance.KustomizeOverlay = &original
		require.NoError(t, db.Create(instance).Error)

		instance.Name = util.Ptr("renamed")
		err := db.Save(instance).Error
		require.NoError(t, err)
	})
}
