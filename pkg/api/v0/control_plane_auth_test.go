package v0

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	gorm "gorm.io/gorm"

	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAuthCanOnlyBeDisabledOnKind covers the rule itself.
//
// With auth disabled the API is served over plain HTTP with no client
// certificate, so anything that can reach it owns the control plane. On kind
// that is a local cluster on the operator's own machine. Anywhere else it is
// a real cluster holding every credential the control plane manages.
func TestAuthCanOnlyBeDisabledOnKind(t *testing.T) {
	assert.NoError(
		t, ValidateAuthEnabledForProvider(KubernetesRuntimeInfraProviderKind, false),
		"kind is the case the flag exists for",
	)

	for _, provider := range []string{
		KubernetesRuntimeInfraProviderEKS,
		KubernetesRuntimeInfraProviderGKE,
	} {
		err := ValidateAuthEnabledForProvider(provider, false)
		require.Error(t, err, "%s must not serve the API without auth", provider)
		assert.Contains(t, err.Error(), provider)
		assert.Contains(t, err.Error(), KubernetesRuntimeInfraProviderKind, "the error says where it is allowed")

		var httpError *util.HttpError
		require.ErrorAs(t, err, &httpError)
		assert.Equal(
			t, http.StatusBadRequest, httpError.StatusCode,
			"a rejected combination is the caller's error, not the server's",
		)

		assert.NoError(
			t, ValidateAuthEnabledForProvider(provider, true),
			"%s runs with auth on, which is the default", provider,
		)
	}
}

// setupControlPlaneAuthDB gives the instance hook the rows it reads.
func setupControlPlaneAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	// the runtime instance encrypts fields on write
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&ControlPlaneDefinition{},
		&ControlPlaneInstance{},
		&KubernetesRuntimeDefinition{},
		&KubernetesRuntimeInstance{},
		&AttachedObjectReference{},
	))

	return db
}

// runtimeOn creates a runtime definition and instance for one provider and
// returns the instance's id.
func runtimeOn(t *testing.T, db *gorm.DB, provider, name string) *uint {
	t.Helper()
	definition := &KubernetesRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr(name + "-definition")},
		InfraProvider: util.Ptr(provider),
	}
	require.NoError(t, db.Create(definition).Error)

	instance := &KubernetesRuntimeInstance{
		Instance: Instance{Name: util.Ptr(name)},
		// KubernetesRuntimeInstance.beforeCreate dereferences Location
		// without checking it, so an instance without one panics rather than
		// being rejected. That is its own problem; this is just not tripping
		// over it.
		Location:                      util.Ptr("Local"),
		KubernetesRuntimeDefinitionID: definition.ID,
	}
	require.NoError(t, db.Create(instance).Error)

	return instance.ID
}

// controlPlaneDefinitionWithAuth creates a definition and returns its id.
func controlPlaneDefinitionWithAuth(t *testing.T, db *gorm.DB, name string, authEnabled bool) *uint {
	t.Helper()
	definition := &ControlPlaneDefinition{
		Definition:  Definition{Name: util.Ptr(name)},
		AuthEnabled: util.Ptr(authEnabled),
	}
	require.NoError(t, db.Create(definition).Error)

	return definition.ID
}

// TestTheApiRefusesAnUnauthenticatedControlPlaneOffKind covers the half the
// CLI cannot.
//
// The CLI check is a rail on one path. A control plane can be created by
// posting to the API, which is how a child control plane is created, so a
// check that only exists in tptctl is one the API is the way around.
func TestTheApiRefusesAnUnauthenticatedControlPlaneOffKind(t *testing.T) {
	t.Run("auth off on a cloud provider is refused", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, "eks-runtime")
		definition := controlPlaneDefinitionWithAuth(t, db, "cp-no-auth", false)

		err := db.Create(&ControlPlaneInstance{
			Instance:                    Instance{Name: util.Ptr("cp-no-auth")},
			Namespace:                   util.Ptr("threeport-control-plane"),
			ControlPlaneDefinitionID:    definition,
			KubernetesRuntimeInstanceID: runtime,
		}).Error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth cannot be disabled")
	})

	t.Run("auth off on kind is allowed", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderKind, "kind-runtime")
		definition := controlPlaneDefinitionWithAuth(t, db, "cp-local", false)

		assert.NoError(t, db.Create(&ControlPlaneInstance{
			Instance:                    Instance{Name: util.Ptr("cp-local")},
			Namespace:                   util.Ptr("threeport-control-plane"),
			ControlPlaneDefinitionID:    definition,
			KubernetesRuntimeInstanceID: runtime,
		}).Error, "local development is what the flag is for")
	})

	t.Run("auth on is allowed anywhere", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, "eks-runtime")
		definition := controlPlaneDefinitionWithAuth(t, db, "cp-authed", true)

		assert.NoError(t, db.Create(&ControlPlaneInstance{
			Instance:                    Instance{Name: util.Ptr("cp-authed")},
			Namespace:                   util.Ptr("threeport-control-plane"),
			ControlPlaneDefinitionID:    definition,
			KubernetesRuntimeInstanceID: runtime,
		}).Error)
	})

	t.Run("a definition whose auth column is null takes the default", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, "eks-runtime")

		// the column defaults to true, so a definition created through the
		// model never arrives here as null. Reaching this branch takes a row
		// written around the default - a migration, or raw SQL - and the
		// answer for an unknown value is the one the default would have
		// given rather than a refusal.
		require.NoError(t, db.Exec(
			"INSERT INTO v0_control_plane_definitions (name, auth_enabled) VALUES (?, NULL)",
			"cp-null-auth",
		).Error)
		var stored ControlPlaneDefinition
		require.NoError(t, db.Where("name = ?", "cp-null-auth").First(&stored).Error)
		require.Nil(t, stored.AuthEnabled, "the row has to be null for this to test anything")

		assert.NoError(t, db.Create(&ControlPlaneInstance{
			Instance:                    Instance{Name: util.Ptr("cp-null-auth")},
			Namespace:                   util.Ptr("threeport-control-plane"),
			ControlPlaneDefinitionID:    stored.ID,
			KubernetesRuntimeInstanceID: runtime,
		}).Error)
	})

	t.Run("a definition that does not mention auth takes the default", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, "eks-runtime")
		definition := &ControlPlaneDefinition{Definition: Definition{Name: util.Ptr("cp-default")}}
		require.NoError(t, db.Create(definition).Error)

		assert.NoError(t, db.Create(&ControlPlaneInstance{
			Instance:                    Instance{Name: util.Ptr("cp-default")},
			Namespace:                   util.Ptr("threeport-control-plane"),
			ControlPlaneDefinitionID:    definition.ID,
			KubernetesRuntimeInstanceID: runtime,
		}).Error, "the column default is auth on, so an unset field is not a refusal")
	})
}

// instanceOf creates a control plane instance bound to a definition and a
// runtime, bypassing the create hook so an update can be tested against a
// pairing that the create path would have refused.
func instanceOf(t *testing.T, db *gorm.DB, name string, definitionID, runtimeID *uint) {
	t.Helper()
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&ControlPlaneInstance{
		Instance:                    Instance{Name: util.Ptr(name)},
		Namespace:                   util.Ptr("threeport-control-plane"),
		ControlPlaneDefinitionID:    definitionID,
		KubernetesRuntimeInstanceID: runtimeID,
	}).Error)
}

// TestAuthCannotBeTurnedOffAfterTheInstanceExists covers the window between
// creating an instance and installing it.
//
// The create hook checks the pairing when the instance is created, and the
// control-plane-controller reads the definition again when it installs. Those
// are different moments. An instance created on EKS with auth on passes the
// first check; patching its definition to auth off before the controller
// reaches it means the install reads the changed value and brings the child
// up without auth, with every check it passed still true of when it passed.
func TestAuthCannotBeTurnedOffAfterTheInstanceExists(t *testing.T) {
	// the shape a PATCH reaches GORM as
	patch := func(db *gorm.DB, stored *ControlPlaneDefinition, incoming *ControlPlaneDefinition) error {
		return db.Model(stored).Updates(incoming).Error
	}

	t.Run("an instance on a cloud provider refuses the update", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, "eks-runtime")
		definitionID := controlPlaneDefinitionWithAuth(t, db, "cp", true)
		instanceOf(t, db, "cp", definitionID, runtime)

		var stored ControlPlaneDefinition
		require.NoError(t, db.First(&stored, *definitionID).Error)

		err := patch(db, &stored, &ControlPlaneDefinition{AuthEnabled: util.Ptr(false)})
		require.Error(t, err, "this is the bypass: create with auth, then patch it away")
		assert.Contains(t, err.Error(), "auth cannot be disabled")
	})

	t.Run("an instance on kind allows it", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderKind, "kind-runtime")
		definitionID := controlPlaneDefinitionWithAuth(t, db, "cp", true)
		instanceOf(t, db, "cp", definitionID, runtime)

		var stored ControlPlaneDefinition
		require.NoError(t, db.First(&stored, *definitionID).Error)

		assert.NoError(
			t, patch(db, &stored, &ControlPlaneDefinition{AuthEnabled: util.Ptr(false)}),
			"local development can still turn it off",
		)
	})

	t.Run("a definition with no instances allows it", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		definitionID := controlPlaneDefinitionWithAuth(t, db, "cp", true)

		var stored ControlPlaneDefinition
		require.NoError(t, db.First(&stored, *definitionID).Error)

		assert.NoError(
			t, patch(db, &stored, &ControlPlaneDefinition{AuthEnabled: util.Ptr(false)}),
			"nothing runs from it yet, and the create hook covers whatever does later",
		)
	})

	t.Run("an update that does not mention auth is left alone", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, "eks-runtime")
		definitionID := controlPlaneDefinitionWithAuth(t, db, "cp", true)
		instanceOf(t, db, "cp", definitionID, runtime)

		var stored ControlPlaneDefinition
		require.NoError(t, db.First(&stored, *definitionID).Error)

		assert.NoError(
			t, patch(db, &stored, &ControlPlaneDefinition{OnboardParent: util.Ptr(false)}),
			"an update setting something else is not held to a value it is not writing",
		)
	})

	t.Run("turning auth on is always allowed", func(t *testing.T) {
		db := setupControlPlaneAuthDB(t)
		runtime := runtimeOn(t, db, KubernetesRuntimeInfraProviderEKS, "eks-runtime")
		definitionID := controlPlaneDefinitionWithAuth(t, db, "cp", true)
		instanceOf(t, db, "cp", definitionID, runtime)

		var stored ControlPlaneDefinition
		require.NoError(t, db.First(&stored, *definitionID).Error)

		assert.NoError(t, patch(db, &stored, &ControlPlaneDefinition{AuthEnabled: util.Ptr(true)}))
	})
}
