package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	testGkeSecretsKey  = "projects/p/locations/us-east1/keyRings/r/cryptoKeys/secrets"
	testGkeBootDiskKey = "projects/p/locations/us-east1/keyRings/r/cryptoKeys/disks"
)

// gkeTestRuntimeDefinitions counts the kubernetes runtime definitions the
// GKE definitions newGkeDefinition returns are attached to, one each.
var gkeTestRuntimeDefinitions uint

// newGkeDefinition returns a GKE definition with the settings set changes
// to it.
func newGkeDefinition(name string, set func(*GcpGkeKubernetesRuntimeDefinition)) *GcpGkeKubernetesRuntimeDefinition {
	gkeTestRuntimeDefinitions++
	definition := &GcpGkeKubernetesRuntimeDefinition{
		Definition: Definition{Name: util.Ptr(name)}, ZoneCount: util.Ptr(1), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
		DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		KubernetesRuntimeDefinitionID: util.Ptr(gkeTestRuntimeDefinitions),
	}
	if set != nil {
		set(definition)
	}

	return definition
}

// TestAGkeDefinitionsNodeSecurityIsChecked covers creating a GKE definition
// with each of its encryption and boot settings: keys that are Cloud KMS key
// names, a disk type and size GKE gives nodes, and a boot disk key only on a
// standard or SSD disk, which an unset type, the balanced disk a new node
// pool gets, isn't.
func TestAGkeDefinitionsNodeSecurityIsChecked(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)

	for name, set := range map[string]func(*GcpGkeKubernetesRuntimeDefinition){
		"none":          nil,
		"a secrets key": func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecretsEncryptionKey = util.Ptr(testGkeSecretsKey) },
		"an ssd":        func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskType = util.Ptr("pd-ssd") },
		"the smallest":  func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskSizeGb = util.Ptr(10) },
		"the largest":   func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskSizeGb = util.Ptr(65536) },
		"no keys": func(g *GcpGkeKubernetesRuntimeDefinition) {
			g.SecretsEncryptionKey, g.BootDiskKmsKey = util.Ptr(""), util.Ptr("")
		},
		"secure boot": func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecureBoot = util.Ptr(true) },
		"intranode":   func(g *GcpGkeKubernetesRuntimeDefinition) { g.IntranodeVisibility = util.Ptr(true) },
		"a key on a ssd": func(g *GcpGkeKubernetesRuntimeDefinition) {
			g.BootDiskKmsKey, g.BootDiskType = util.Ptr(testGkeBootDiskKey), util.Ptr("pd-ssd")
		},
		"a key on a standard disk": func(g *GcpGkeKubernetesRuntimeDefinition) {
			g.BootDiskKmsKey, g.BootDiskType = util.Ptr(testGkeBootDiskKey), util.Ptr("pd-standard")
		},
	} {
		assert.NoError(t, db.Create(newGkeDefinition("ok-"+name, set)).Error, name)
	}

	for want, set := range map[string]func(*GcpGkeKubernetesRuntimeDefinition){
		`SecretsEncryptionKey "my-key" is not a Cloud KMS key's resource name`: func(g *GcpGkeKubernetesRuntimeDefinition) {
			g.SecretsEncryptionKey = util.Ptr("my-key")
		},
		`BootDiskKmsKey "projects/p/locations/l/keyRings/r" is not a Cloud KMS key's resource name`: func(g *GcpGkeKubernetesRuntimeDefinition) {
			g.BootDiskKmsKey, g.BootDiskType = util.Ptr("projects/p/locations/l/keyRings/r"), util.Ptr("pd-ssd")
		},
		`BootDiskType "hyperdisk-balanced" is not one of [pd-standard pd-balanced pd-ssd]`: func(g *GcpGkeKubernetesRuntimeDefinition) {
			g.BootDiskType = util.Ptr("hyperdisk-balanced")
		},
		"BootDiskSizeGb 9 is not from 10 to 65536":     func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskSizeGb = util.Ptr(9) },
		"BootDiskSizeGb 65537 is not from 10 to 65536": func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskSizeGb = util.Ptr(65537) },
		"BootDiskKmsKey needs a BootDiskType of [pd-standard pd-ssd]: GKE doesn't encrypt a pd-balanced boot disk with a key of yours": func(g *GcpGkeKubernetesRuntimeDefinition) {
			g.BootDiskKmsKey = util.Ptr(testGkeBootDiskKey)
		},
	} {
		assert.ErrorContains(t, db.Create(newGkeDefinition("refused", set)).Error, want)
	}
	refused := newGkeDefinition("refused-balanced", func(g *GcpGkeKubernetesRuntimeDefinition) {
		g.BootDiskKmsKey, g.BootDiskType = util.Ptr(testGkeBootDiskKey), util.Ptr("pd-balanced")
	})
	assert.ErrorContains(t, db.Create(refused).Error, "doesn't encrypt a pd-balanced boot disk", "a balanced disk asked for")
}

// TestAGkeDefinitionsNodeSecurityIsCheckedOnUpdate covers updating a GKE
// definition's encryption and boot settings: each update is checked with the
// settings it leaves the definition with, so a boot disk key and a balanced
// disk are refused whichever comes first, and a secrets key can be rotated
// but not cleared, on either update shape.
func TestAGkeDefinitionsNodeSecurityIsCheckedOnUpdate(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	session := func() *gorm.DB { return db.Session(&gorm.Session{}) }
	created := newGkeDefinition("gke", nil)
	require.NoError(t, db.Create(created).Error)
	load := func() *GcpGkeKubernetesRuntimeDefinition {
		var loaded GcpGkeKubernetesRuntimeDefinition
		require.NoError(t, db.First(&loaded, *created.ID).Error)
		return &loaded
	}
	patch := func(set func(*GcpGkeKubernetesRuntimeDefinition)) error {
		update := &GcpGkeKubernetesRuntimeDefinition{}
		set(update)
		return session().Model(load()).Updates(update).Error
	}

	assert.ErrorContains(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskKmsKey = util.Ptr(testGkeBootDiskKey) }),
		"doesn't encrypt a pd-balanced boot disk", "a key on the balanced disk a type left out is")
	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskType = util.Ptr("pd-ssd") }))
	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskKmsKey = util.Ptr(testGkeBootDiskKey) }),
		"a key on the ssd the definition has")
	assert.ErrorContains(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskType = util.Ptr("pd-balanced") }),
		"doesn't encrypt a pd-balanced boot disk", "a balanced disk under the key it has")
	assert.ErrorContains(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskSizeGb = util.Ptr(5) }), "BootDiskSizeGb 5")

	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecretsEncryptionKey = util.Ptr(testGkeSecretsKey) }))
	rotated := "projects/p/locations/us-east1/keyRings/r/cryptoKeys/secrets-2"
	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecretsEncryptionKey = util.Ptr(rotated) }), "a rotation")
	assert.ErrorContains(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecretsEncryptionKey = util.Ptr("") }),
		ErrMsgSecretsEncryptionKeyCleared, "a patch")
	cleared := load()
	cleared.SecretsEncryptionKey = nil
	assert.ErrorContains(t, session().Save(cleared).Error, ErrMsgSecretsEncryptionKeyCleared, "a replace")

	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.DefaultNodeGroupMaximumSize = util.Ptr(5) }),
		"an update of other fields")
	require.NoError(t, session().Save(load()).Error, "a replace that keeps them")

	stored := load()
	assert.Equal(t, rotated, *stored.SecretsEncryptionKey)
	assert.Equal(t, testGkeBootDiskKey, *stored.BootDiskKmsKey)
	assert.Equal(t, "pd-ssd", *stored.BootDiskType)
	assert.Nil(t, stored.BootDiskSizeGb)
}

// TestAProtectedGkeDefinitionsNodePoolsArentReplaced covers an update that
// changes a boot disk key or Secure Boot, which replaces the node pools of
// the definition's clusters: refused while DeletionProtection protects them,
// and allowed once it doesn't, or for a definition with no clusters yet.  A
// change that doesn't replace them goes through.
func TestAProtectedGkeDefinitionsNodePoolsArentReplaced(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	require.NoError(t, db.AutoMigrate(&GcpGkeKubernetesRuntimeInstance{}))
	session := func() *gorm.DB { return db.Session(&gorm.Session{}) }

	protected := newGkeDefinition("protected", func(g *GcpGkeKubernetesRuntimeDefinition) {
		g.DeletionProtection = util.Ptr(true)
		g.BootDiskType = util.Ptr("pd-ssd")
	})
	require.NoError(t, db.Create(protected).Error)
	patch := func(set func(*GcpGkeKubernetesRuntimeDefinition)) error {
		var loaded GcpGkeKubernetesRuntimeDefinition
		require.NoError(t, db.First(&loaded, *protected.ID).Error)
		update := &GcpGkeKubernetesRuntimeDefinition{}
		set(update)
		return session().Model(&loaded).Updates(update).Error
	}

	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecureBoot = util.Ptr(true) }), "no clusters yet")
	require.NoError(t, db.Create(&GcpGkeKubernetesRuntimeInstance{
		Instance:                            Instance{Name: util.Ptr("gke")},
		GcpProviderID:                       util.Ptr(uint(1)),
		GcpGkeKubernetesRuntimeDefinitionID: protected.ID,
		KubernetesRuntimeInstanceID:         util.Ptr(uint(1)),
	}).Error)

	const want = "changing BootDiskKmsKey or SecureBoot replaces the node pools of the definition's clusters, which DeletionProtection protects"
	assert.ErrorContains(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecureBoot = util.Ptr(false) }), want)
	assert.ErrorContains(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.BootDiskKmsKey = util.Ptr(testGkeBootDiskKey) }), want)
	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecureBoot = util.Ptr(true) }), "the Secure Boot it has")
	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) {
		g.BootDiskSizeGb, g.IntranodeVisibility, g.SecretsEncryptionKey = util.Ptr(200), util.Ptr(true), util.Ptr(testGkeSecretsKey)
	}), "settings that keep the node pools")

	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) {
		g.DeletionProtection = util.Ptr(false)
		g.BootDiskKmsKey = util.Ptr(testGkeBootDiskKey)
	}), "protection turned off in the same update")
	require.NoError(t, patch(func(g *GcpGkeKubernetesRuntimeDefinition) { g.SecureBoot = util.Ptr(false) }), "or before it")
}

// TestGkePendingSettingsAreThoseAClusterDoesntRun covers the settings a
// GKE definition asks for that its instance's cluster doesn't run: each set
// on the definition and recorded otherwise on the instance, and each set to
// other than GKE's default on a cluster that records none, as one created
// before they were recorded.  Those the definition leaves unset ask for
// nothing.
func TestGkePendingSettingsAreThoseAClusterDoesntRun(t *testing.T) {
	applied := &GcpGkeKubernetesRuntimeInstance{
		SecretsEncryptionKey: util.Ptr(""),
		BootDiskType:         util.Ptr("pd-balanced"),
		BootDiskSizeGb:       util.Ptr(100),
		BootDiskKmsKey:       util.Ptr(""),
		SecureBoot:           util.Ptr(false),
		IntranodeVisibility:  util.Ptr(false),
	}

	assert.Empty(t, GkePendingSettings(newGkeDefinition("gke", nil), applied), "nothing asked")
	assert.Empty(t, GkePendingSettings(newGkeDefinition("gke", nil), &GcpGkeKubernetesRuntimeInstance{}), "nothing asked of an older cluster")
	same := newGkeDefinition("gke", func(g *GcpGkeKubernetesRuntimeDefinition) {
		g.SecretsEncryptionKey, g.BootDiskType, g.BootDiskSizeGb = util.Ptr(""), util.Ptr("pd-balanced"), util.Ptr(100)
		g.BootDiskKmsKey, g.SecureBoot, g.IntranodeVisibility = util.Ptr(""), util.Ptr(false), util.Ptr(false)
	})
	assert.Empty(t, GkePendingSettings(same, applied), "what it runs")

	every := newGkeDefinition("gke", func(g *GcpGkeKubernetesRuntimeDefinition) {
		g.SecretsEncryptionKey, g.BootDiskType, g.BootDiskSizeGb = util.Ptr(testGkeSecretsKey), util.Ptr("pd-ssd"), util.Ptr(200)
		g.BootDiskKmsKey, g.SecureBoot, g.IntranodeVisibility = util.Ptr(testGkeBootDiskKey), util.Ptr(true), util.Ptr(true)
	})
	all := []string{"SecretsEncryptionKey", "BootDiskType", "BootDiskSizeGb", "BootDiskKmsKey", "SecureBoot", "IntranodeVisibility"}
	assert.Equal(t, all, GkePendingSettings(every, applied))
	assert.Equal(t, all, GkePendingSettings(every, &GcpGkeKubernetesRuntimeInstance{}), "an older cluster")

	assert.Equal(t, []string{"BootDiskType", "BootDiskSizeGb"}, GkePendingSettings(same, &GcpGkeKubernetesRuntimeInstance{}),
		"an older cluster's disk isn't known, and its keys, Secure Boot and intranode visibility are GKE's defaults")
}
