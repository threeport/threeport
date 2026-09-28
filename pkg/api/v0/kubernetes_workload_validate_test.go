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

// TestValidateKustomizeOverlaySize_Update covers the two GORM call shapes a
// PATCH/PUT can take (see pkg/api/lib/v0/update_helpers.go): Updates
// (partial, PATCH) and Save (full replace, PUT). Both must read the inbound
// KustomizeOverlay via lib.IncomingValues, not the stored row, or an
// oversized patch would slip through validation.
func TestValidateKustomizeOverlaySize_Update(t *testing.T) {
	oversized := strings.Repeat("a", MaxKustomizeOverlayBytes+1)
	withinLimit := strings.Repeat("a", 10)

	t.Run("PATCH (Updates) rejects an oversized overlay", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		instance.KustomizeOverlay = &withinLimit
		require.NoError(t, db.Create(instance).Error)

		err := db.Model(instance).
			Updates(&KubernetesWorkloadInstance{KustomizeOverlay: &oversized}).Error
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the")
	})

	t.Run("PATCH (Updates) leaving KustomizeOverlay unset is unaffected by an existing oversized value", func(t *testing.T) {
		// KustomizeOverlay can only reach an oversized stored value via a
		// bug elsewhere (this validation runs on every write), but the
		// hook must still key off the inbound patch, not the stored row,
		// so an update to an unrelated field never trips over a
		// pre-existing value it isn't touching.
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		require.NoError(t, db.Create(instance).Error)

		newName := "renamed"
		err := db.Model(instance).
			Updates(&KubernetesWorkloadInstance{Instance: Instance{Name: &newName}}).Error
		require.NoError(t, err)
	})

	t.Run("PUT (Save) rejects an oversized overlay", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		instance.KustomizeOverlay = &withinLimit
		require.NoError(t, db.Create(instance).Error)

		instance.KustomizeOverlay = &oversized
		err := db.Save(instance).Error
		require.Error(t, err)
		require.Contains(t, err.Error(), "exceeds the")
	})

	t.Run("PUT (Save) accepts an overlay within the limit", func(t *testing.T) {
		db := setupKubernetesWorkloadInstanceDB(t)
		instance := newTestKubernetesWorkloadInstance()
		require.NoError(t, db.Create(instance).Error)

		instance.KustomizeOverlay = &withinLimit
		err := db.Save(instance).Error
		require.NoError(t, err)
	})
}
