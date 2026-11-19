package gcp

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestBuildGkeInfra_PassesTheNodeSecurity covers buildGkeInfra handing the
// program the definition's secrets encryption, boot disk, Secure Boot and
// intranode visibility, and leaving each the definition doesn't set unset.
func TestBuildGkeInfra_PassesTheNodeSecurity(t *testing.T) {
	projectID := "some-project"
	gcpProviderID := uint(1)
	credentials := `{"type":"service_account","project_id":"some-project","client_email":"threeport@some-project.iam.gserviceaccount.com"}`
	encryptionKey, storedCredentials := encryptedCredentials(t, credentials)
	srv := gkeProviderServer(t, api_v0.GcpProvider{
		Common:                    api_v0.Common{ID: &gcpProviderID},
		ProjectID:                 &projectID,
		ServiceAccountCredentials: &storedCredentials,
	})
	defer srv.Close()
	build := func(definition *api_v0.GcpGkeKubernetesRuntimeDefinition) *provider.KubernetesRuntimeInfraGKE {
		infra, err := buildGkeInfra(
			&controller.Reconciler{
				APIClient:     &http.Client{},
				APIServer:     strings.TrimPrefix(srv.URL, "http://"),
				EncryptionKey: encryptionKey,
			},
			testGkeInstance(&gcpProviderID),
			definition,
			nil,
		)
		require.NoError(t, err)
		return infra
	}

	infra := build(testGkeDefinition())
	assert.Nil(t, infra.SecretsEncryptionKey)
	assert.Nil(t, infra.BootDiskType)
	assert.Nil(t, infra.BootDiskSizeGb)
	assert.Nil(t, infra.BootDiskKmsKey)
	assert.Nil(t, infra.SecureBoot)
	assert.Nil(t, infra.IntranodeVisibility)

	definition := testGkeDefinition()
	definition.SecretsEncryptionKey = util.Ptr("projects/p/locations/l/keyRings/r/cryptoKeys/secrets")
	definition.BootDiskType = util.Ptr("pd-ssd")
	definition.BootDiskSizeGb = util.Ptr(200)
	definition.BootDiskKmsKey = util.Ptr("projects/p/locations/l/keyRings/r/cryptoKeys/disks")
	definition.SecureBoot = util.Ptr(true)
	definition.IntranodeVisibility = util.Ptr(false)
	infra = build(definition)
	assert.Equal(t, definition.SecretsEncryptionKey, infra.SecretsEncryptionKey)
	assert.Equal(t, definition.BootDiskType, infra.BootDiskType)
	assert.Equal(t, definition.BootDiskSizeGb, infra.BootDiskSizeGb)
	assert.Equal(t, definition.BootDiskKmsKey, infra.BootDiskKmsKey)
	assert.Equal(t, definition.SecureBoot, infra.SecureBoot)
	assert.Equal(t, definition.IntranodeVisibility, infra.IntranodeVisibility)
}

// TestBuildGkeInfra_KeepsWhatTheClusterRunsForSettingsLeftOut covers a
// definition updated without settings its cluster runs: the program keeps
// them, so a later upgrade doesn't replace a node pool with a boot disk key
// and Secure Boot by one without them.  An explicit false or "" turns one
// off, and a setting the cluster runs as its default stays unset.
func TestBuildGkeInfra_KeepsWhatTheClusterRunsForSettingsLeftOut(t *testing.T) {
	projectID := "some-project"
	gcpProviderID := uint(1)
	credentials := `{"type":"service_account","project_id":"some-project","client_email":"threeport@some-project.iam.gserviceaccount.com"}`
	encryptionKey, storedCredentials := encryptedCredentials(t, credentials)
	srv := gkeProviderServer(t, api_v0.GcpProvider{
		Common:                    api_v0.Common{ID: &gcpProviderID},
		ProjectID:                 &projectID,
		ServiceAccountCredentials: &storedCredentials,
	})
	defer srv.Close()
	build := func(
		definition *api_v0.GcpGkeKubernetesRuntimeDefinition,
		instance *api_v0.GcpGkeKubernetesRuntimeInstance,
	) *provider.KubernetesRuntimeInfraGKE {
		infra, err := buildGkeInfra(
			&controller.Reconciler{
				APIClient:     &http.Client{},
				APIServer:     strings.TrimPrefix(srv.URL, "http://"),
				EncryptionKey: encryptionKey,
			},
			instance,
			definition,
			nil,
		)
		require.NoError(t, err)
		return infra
	}
	secured := func() *api_v0.GcpGkeKubernetesRuntimeInstance {
		instance := testGkeInstance(&gcpProviderID)
		instance.SecretsEncryptionKey = util.Ptr("projects/p/locations/l/keyRings/r/cryptoKeys/secrets")
		instance.BootDiskType = util.Ptr("pd-ssd")
		instance.BootDiskSizeGb = util.Ptr(60)
		instance.BootDiskKmsKey = util.Ptr("projects/p/locations/l/keyRings/r/cryptoKeys/disks")
		instance.SecureBoot = util.Ptr(true)
		instance.IntranodeVisibility = util.Ptr(true)
		return instance
	}

	instance := secured()
	infra := build(testGkeDefinition(), instance)
	assert.Equal(t, instance.SecretsEncryptionKey, infra.SecretsEncryptionKey)
	assert.Equal(t, instance.BootDiskType, infra.BootDiskType)
	assert.Equal(t, instance.BootDiskSizeGb, infra.BootDiskSizeGb)
	assert.Equal(t, instance.BootDiskKmsKey, infra.BootDiskKmsKey)
	assert.Equal(t, instance.SecureBoot, infra.SecureBoot)
	assert.Equal(t, instance.IntranodeVisibility, infra.IntranodeVisibility)

	definition := testGkeDefinition()
	definition.BootDiskKmsKey = util.Ptr("")
	definition.SecureBoot = util.Ptr(false)
	definition.IntranodeVisibility = util.Ptr(false)
	infra = build(definition, secured())
	assert.Equal(t, util.Ptr(""), infra.BootDiskKmsKey, "turned off")
	assert.Equal(t, util.Ptr(false), infra.SecureBoot, "turned off")
	assert.Equal(t, util.Ptr(false), infra.IntranodeVisibility, "turned off")
	assert.Equal(t, util.Ptr("pd-ssd"), infra.BootDiskType, "kept")

	instance = testGkeInstance(&gcpProviderID)
	instance.SecretsEncryptionKey = util.Ptr("")
	instance.BootDiskKmsKey = util.Ptr("")
	instance.SecureBoot = util.Ptr(false)
	instance.IntranodeVisibility = util.Ptr(false)
	infra = build(testGkeDefinition(), instance)
	assert.Nil(t, infra.SecretsEncryptionKey)
	assert.Nil(t, infra.BootDiskKmsKey)
	assert.Nil(t, infra.SecureBoot)
	assert.Nil(t, infra.IntranodeVisibility)
}

// TestTheGkeInstanceRecordsTheSettingsItRuns covers the settings the GKE
// adapter reads and records: those of its definition the cluster doesn't
// run, as the instance records them, and those a step applied, each written
// to the instance, the keys as "" when there are none.
func TestTheGkeInstanceRecordsTheSettingsItRuns(t *testing.T) {
	instance := api_v0.GcpGkeKubernetesRuntimeInstance{
		Common:                              api_v0.Common{ID: util.Ptr(uint(7))},
		GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
		BootDiskType:                        util.Ptr("pd-balanced"),
		SecureBoot:                          util.Ptr(false),
	}
	definition := api_v0.GcpGkeKubernetesRuntimeDefinition{
		Common:       api_v0.Common{ID: util.Ptr(uint(3))},
		BootDiskType: util.Ptr("pd-ssd"),
		SecureBoot:   util.Ptr(true),
	}
	lifecycle, updates := gkeUpgradeAPI(t, instance, definition)

	pending, err := lifecycle.PendingSettings()
	require.NoError(t, err)
	assert.Equal(t, []string{"BootDiskType", "SecureBoot"}, pending)

	require.NoError(t, lifecycle.OnSettingsApplied(&provider.KubernetesRuntimeInfraGKE{
		BootDiskType: util.Ptr("pd-ssd"),
		SecureBoot:   util.Ptr(true),
	}))
	sent := updates()
	require.Len(t, sent, 1)
	assert.Equal(t, map[string]any{
		"SecretsEncryptionKey": "",
		"BootDiskType":         "pd-ssd",
		"BootDiskSizeGb":       float64(100),
		"BootDiskKmsKey":       "",
		"SecureBoot":           true,
		"IntranodeVisibility":  false,
	}, sent[0])

	assert.Error(t, lifecycle.OnSettingsApplied(nil), "an infra that isn't GKE's")
}
