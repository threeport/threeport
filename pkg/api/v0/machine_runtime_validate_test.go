package v0

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// newValidMRI returns a MachineRuntimeInstance with the minimum valid
// fields: a name, hostname, ssh user, and an ssh password credential.
func newValidMRI(name string) *MachineRuntimeInstance {
	return &MachineRuntimeInstance{
		Instance:    Instance{Name: util.Ptr(name)},
		Hostname:    util.Ptr("host.example"),
		SSHUser:     util.Ptr("user"),
		SSHPassword: util.Ptr("password"),
	}
}

// createProvisionedMRI seeds an MRI with every provisioning identity field
// set and returns the row reloaded from DB, mirroring the PATCH handler's
// load-then-update flow.
func createProvisionedMRI(t *testing.T, db *gorm.DB, name string) MachineRuntimeInstance {
	t.Helper()
	mri := newValidMRI(name)
	mri.InfraProvider = util.Ptr("gce")
	mri.Region = util.Ptr("us-central1")
	mri.MachineType = util.Ptr("e2-medium")
	mri.ImageID = util.Ptr("image-1")
	mri.NetworkID = util.Ptr("network-1")
	require.NoError(t, db.Create(mri).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)
	return loaded
}

// TestMachineRuntimeInstance_BeforeCreate_RequiresCredential rejects an MRI
// with neither SSHKey nor SSHPassword; the reconciler needs a credential to
// authenticate with the machine.
func TestMachineRuntimeInstance_BeforeCreate_RequiresCredential(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-no-cred")
	mri.SSHPassword = nil

	err := db.Create(mri).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of SSHKey or SSHPassword")
}

// TestMachineRuntimeInstance_BeforeCreate_ImportedMachinePasses accepts an
// MRI with a credential and no infra provider or region, proving the
// provider-requires-region rule does not fire for imported machines.
func TestMachineRuntimeInstance_BeforeCreate_ImportedMachinePasses(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-imported")
	require.NoError(t, db.Create(mri).Error)
}

// TestMachineRuntimeInstance_BeforeCreate_ProviderRequiresRegion rejects an
// MRI that sets an infra provider without a region; no provider can
// provision a machine without one.
func TestMachineRuntimeInstance_BeforeCreate_ProviderRequiresRegion(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-provider-no-region")
	mri.InfraProvider = util.Ptr("gce")

	err := db.Create(mri).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must also set a region")
}

// TestMachineRuntimeInstance_BeforeUpdate_ProviderFieldsImmutable seeds a
// fully provisioned MRI and asserts every provisioning identity field is
// rejected on update through a live update tx.
func TestMachineRuntimeInstance_BeforeUpdate_ProviderFieldsImmutable(t *testing.T) {
	tests := []struct {
		name    string
		payload *MachineRuntimeInstance
	}{
		{"infra provider", &MachineRuntimeInstance{InfraProvider: util.Ptr("other")}},
		{"region", &MachineRuntimeInstance{Region: util.Ptr("other-region")}},
		{"machine type", &MachineRuntimeInstance{MachineType: util.Ptr("other-type")}},
		{"image id", &MachineRuntimeInstance{ImageID: util.Ptr("other-image")}},
		{"network id", &MachineRuntimeInstance{NetworkID: util.Ptr("other-network")}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupMachineWorkloadValidateDB(t)
			loaded := createProvisionedMRI(t, db, fmt.Sprintf("mri-immutable-%d", i))

			err := db.Model(&loaded).Updates(tt.payload).Error
			require.Error(t, err, "changing %s must be rejected", tt.name)
			assert.Contains(t, err.Error(), tt.name+" cannot be changed after creation")
		})
	}
}

// TestMachineRuntimeInstance_BeforeUpdate_ProviderRequiresRegionSemantics
// locks in immutable-once-introduced semantics: a provider cannot be
// attached to an existing machine via update, even when the field was
// unset at create time. Create is the only place the provider/region
// pairing is validated.
func TestMachineRuntimeInstance_BeforeUpdate_ProviderRequiresRegionSemantics(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-late-provider")
	require.NoError(t, db.Create(mri).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)

	err := db.Model(&loaded).Updates(&MachineRuntimeInstance{InfraProvider: util.Ptr("gce")}).Error
	require.Error(t, err, "introducing a provider post-create must be rejected")
	assert.Contains(t, err.Error(), "infra provider cannot be changed after creation")
}

// TestMachineRuntimeInstance_ResourceInventory_RoundTrips guards the JSON
// column under the in-memory sqlite harness: a sqlite/datatypes
// incompatibility would otherwise silently break every test that migrates
// this table.
func TestMachineRuntimeInstance_ResourceInventory_RoundTrips(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	inventory := datatypes.JSON([]byte(`{"vmId":"i-123"}`))
	mri := newValidMRI("mri-inventory")
	mri.InfraProvider = util.Ptr("gce")
	mri.Region = util.Ptr("us-central1")
	mri.ResourceInventory = &inventory
	require.NoError(t, db.Create(mri).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)
	require.NotNil(t, loaded.ResourceInventory)
	assert.JSONEq(t, `{"vmId":"i-123"}`, string(*loaded.ResourceInventory))
}
