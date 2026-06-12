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

// createProvisionedMRI seeds an MRI with every provisioning location field
// set and returns the row reloaded from DB, mirroring the PATCH handler's
// load-then-update flow.
func createProvisionedMRI(t *testing.T, db *gorm.DB, name string) MachineRuntimeInstance {
	t.Helper()
	mri := newValidMRI(name)
	mri.Region = util.Ptr("us-central1")
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
// MRI with a credential and no provisioning location fields, proving an
// imported machine is valid on its own.
func TestMachineRuntimeInstance_BeforeCreate_ImportedMachinePasses(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-imported")
	require.NoError(t, db.Create(mri).Error)
}

// TestMachineRuntimeInstance_BeforeUpdate_LocationFieldsImmutable seeds a
// provisioned MRI and asserts every provisioning location field is rejected
// on update through a live update tx. The provider, machine type, and image
// live on the definition and are guarded by its own immutability test.
func TestMachineRuntimeInstance_BeforeUpdate_LocationFieldsImmutable(t *testing.T) {
	tests := []struct {
		name    string
		payload *MachineRuntimeInstance
	}{
		{"region", &MachineRuntimeInstance{Region: util.Ptr("other-region")}},
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

// TestMachineRuntimeDefinition_BeforeUpdate_TemplateFieldsImmutable seeds a
// definition with every provisioning template field set, then asserts each
// is rejected on update. These fields shape the machines derived from the
// definition, so changing them post-create would diverge running machines.
func TestMachineRuntimeDefinition_BeforeUpdate_TemplateFieldsImmutable(t *testing.T) {
	tests := []struct {
		name    string
		payload *MachineRuntimeDefinition
	}{
		{"infra provider", &MachineRuntimeDefinition{InfraProvider: util.Ptr("other")}},
		{"machine type", &MachineRuntimeDefinition{MachineType: util.Ptr("other-type")}},
		{"image id", &MachineRuntimeDefinition{ImageID: util.Ptr("other-image")}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupMachineWorkloadValidateDB(t)
			mrd := &MachineRuntimeDefinition{
				Definition:    Definition{Name: util.Ptr(fmt.Sprintf("mrd-immutable-%d", i))},
				InfraProvider: util.Ptr("gce"),
				MachineType:   util.Ptr("e2-medium"),
				ImageID:       util.Ptr("image-1"),
			}
			require.NoError(t, db.Create(mrd).Error)

			var loaded MachineRuntimeDefinition
			require.NoError(t, db.First(&loaded, *mrd.ID).Error)

			err := db.Model(&loaded).Updates(tt.payload).Error
			require.Error(t, err, "changing %s must be rejected", tt.name)
			assert.Contains(t, err.Error(), tt.name+" cannot be changed after creation")
		})
	}
}

// TestMachineRuntimeInstance_BeforeCreate_RejectsProviderWithoutRegion asserts
// that creating an instance whose referenced definition has an infra provider
// is rejected when the instance does not supply a region.
func TestMachineRuntimeInstance_BeforeCreate_RejectsProviderWithoutRegion(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mrd := &MachineRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr("mrd-with-provider")},
		InfraProvider: util.Ptr("gce"),
	}
	require.NoError(t, db.Create(mrd).Error)

	mri := newValidMRI("mri-no-region")
	mri.MachineRuntimeDefinitionID = mrd.ID
	// Region intentionally left nil.

	err := db.Create(mri).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must have a region when the definition specifies an infra provider")
}

// TestMachineRuntimeInstance_BeforeCreate_AcceptsProviderWithRegion accepts an
// instance whose referenced definition has an infra provider when the instance
// also supplies a region.
func TestMachineRuntimeInstance_BeforeCreate_AcceptsProviderWithRegion(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mrd := &MachineRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr("mrd-with-provider-2")},
		InfraProvider: util.Ptr("gce"),
	}
	require.NoError(t, db.Create(mrd).Error)

	mri := newValidMRI("mri-with-region")
	mri.MachineRuntimeDefinitionID = mrd.ID
	mri.Region = util.Ptr("us-central1")

	require.NoError(t, db.Create(mri).Error)
}

// TestMachineRuntimeInstance_BeforeCreate_AcceptsNilDefinitionFK accepts an
// instance with no machine runtime definition foreign key set: an imported
// machine does not need to reference a definition at all.
func TestMachineRuntimeInstance_BeforeCreate_AcceptsNilDefinitionFK(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-no-def-fk")
	// MachineRuntimeDefinitionID intentionally left nil.

	require.NoError(t, db.Create(mri).Error)
}

// TestMachineRuntimeInstance_ResourceInventory_RoundTrips guards the JSON
// column under the in-memory sqlite harness: a sqlite/datatypes
// incompatibility would otherwise silently break every test that migrates
// this table.
func TestMachineRuntimeInstance_ResourceInventory_RoundTrips(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	inventory := datatypes.JSON([]byte(`{"vmId":"i-123"}`))
	mri := newValidMRI("mri-inventory")
	mri.Region = util.Ptr("us-central1")
	mri.ResourceInventory = &inventory
	require.NoError(t, db.Create(mri).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)
	require.NotNil(t, loaded.ResourceInventory)
	assert.JSONEq(t, `{"vmId":"i-123"}`, string(*loaded.ResourceInventory))
}
