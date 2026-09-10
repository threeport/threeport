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

// newValidMRI returns an MRI with a name, hostname, ssh user, and ssh password.
func newValidMRI(name string) *MachineRuntimeInstance {
	return &MachineRuntimeInstance{
		Instance:    Instance{Name: util.Ptr(name)},
		Hostname:    util.Ptr("host.example"),
		SSHUser:     util.Ptr("user"),
		SSHPassword: util.Ptr("password"),
	}
}

// createProvisionedMRI seeds an MRI with region, network id, and subnet id set
// and returns the row reloaded from the database.
func createProvisionedMRI(t *testing.T, db *gorm.DB, name string) MachineRuntimeInstance {
	t.Helper()
	mri := newValidMRI(name)
	mri.Region = util.Ptr("us-central1")
	mri.NetworkID = util.Ptr("network-1")
	mri.SubnetID = util.Ptr("subnet-1")
	require.NoError(t, db.Create(mri).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)
	return loaded
}

// TestMachineRuntimeInstance_BeforeCreate_RequiresCredential rejects an MRI
// that has neither SSHKey nor SSHPassword.
func TestMachineRuntimeInstance_BeforeCreate_RequiresCredential(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-no-cred")
	mri.SSHPassword = nil

	err := db.Create(mri).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of SSHKey or SSHPassword")
}

// TestMachineRuntimeInstance_BeforeCreate_ImportedMachinePasses accepts an
// MRI with a credential and no provisioning location fields.
func TestMachineRuntimeInstance_BeforeCreate_ImportedMachinePasses(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-imported")
	require.NoError(t, db.Create(mri).Error)
}

// TestMachineRuntimeInstance_BeforeUpdate_LocationFieldsImmutable rejects an
// update that changes region, network id, or subnet id on a provisioned MRI.
func TestMachineRuntimeInstance_BeforeUpdate_LocationFieldsImmutable(t *testing.T) {
	tests := []struct {
		name    string
		payload *MachineRuntimeInstance
	}{
		{"region", &MachineRuntimeInstance{Region: util.Ptr("other-region")}},
		{"network id", &MachineRuntimeInstance{NetworkID: util.Ptr("other-network")}},
		{"subnet id", &MachineRuntimeInstance{SubnetID: util.Ptr("other-subnet")}},
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

// TestMachineRuntimeDefinition_BeforeUpdate_TemplateFieldsImmutable rejects
// an update that changes infra provider, machine type, or image id.
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

// TestMachineRuntimeInstance_BeforeCreate_RejectsProviderWithoutRegion rejects
// an MRI whose definition has an infra provider when the MRI has no region.
func TestMachineRuntimeInstance_BeforeCreate_RejectsProviderWithoutRegion(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mrd := &MachineRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr("mrd-with-provider")},
		InfraProvider: util.Ptr("gce"),
	}
	require.NoError(t, db.Create(mrd).Error)

	mri := newValidMRI("mri-no-region")
	mri.MachineRuntimeDefinitionID = mrd.ID

	err := db.Create(mri).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must have a region when the definition specifies an infra provider")
}

// TestMachineRuntimeInstance_BeforeCreate_AcceptsProviderWithRegion accepts
// an MRI whose definition has an infra provider when the MRI supplies a region.
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
// MRI with MachineRuntimeDefinitionID unset.
func TestMachineRuntimeInstance_BeforeCreate_AcceptsNilDefinitionFK(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-no-def-fk")

	require.NoError(t, db.Create(mri).Error)
}

// TestMachineRuntimeInstance_ResourceInventory_RoundTrips covers create and reload of ResourceInventory.
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
