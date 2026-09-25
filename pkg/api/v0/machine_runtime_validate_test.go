package v0

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestMachineRuntimeInstance_BeforeUpdate_LocationFieldsImmutableOnSave
// rejects a PUT-shaped Save that changes region, network id, or subnet id.
func TestMachineRuntimeInstance_BeforeUpdate_LocationFieldsImmutableOnSave(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MachineRuntimeInstance)
	}{
		{"region", func(m *MachineRuntimeInstance) { m.Region = util.Ptr("other-region") }},
		{"network id", func(m *MachineRuntimeInstance) { m.NetworkID = util.Ptr("other-network") }},
		{"subnet id", func(m *MachineRuntimeInstance) { m.SubnetID = util.Ptr("other-subnet") }},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupMachineWorkloadValidateDB(t)
			loaded := createProvisionedMRI(t, db, fmt.Sprintf("mri-save-immutable-%d", i))
			tt.mutate(&loaded)

			err := db.Save(&loaded).Error
			require.Error(t, err, "saving a changed %s must be rejected", tt.name)
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

// TestMachineRuntimeDefinition_BeforeUpdate_TemplateFieldsImmutableOnSave
// rejects a PUT-shaped Save that changes infra provider, machine type, or
// image id.
func TestMachineRuntimeDefinition_BeforeUpdate_TemplateFieldsImmutableOnSave(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MachineRuntimeDefinition)
	}{
		{"infra provider", func(m *MachineRuntimeDefinition) { m.InfraProvider = util.Ptr("other") }},
		{"machine type", func(m *MachineRuntimeDefinition) { m.MachineType = util.Ptr("other-type") }},
		{"image id", func(m *MachineRuntimeDefinition) { m.ImageID = util.Ptr("other-image") }},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupMachineWorkloadValidateDB(t)
			mrd := &MachineRuntimeDefinition{
				Definition:    Definition{Name: util.Ptr(fmt.Sprintf("mrd-save-immutable-%d", i))},
				InfraProvider: util.Ptr("gce"),
				MachineType:   util.Ptr("e2-medium"),
				ImageID:       util.Ptr("image-1"),
			}
			require.NoError(t, db.Create(mrd).Error)

			var loaded MachineRuntimeDefinition
			require.NoError(t, db.First(&loaded, *mrd.ID).Error)
			tt.mutate(&loaded)

			err := db.Save(&loaded).Error
			require.Error(t, err, "saving a changed %s must be rejected", tt.name)
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

// TestMachineRuntimeInstance_BeforeCreate_AcceptsNilDefinitionID accepts an
// MRI with MachineRuntimeDefinitionID unset.
func TestMachineRuntimeInstance_BeforeCreate_AcceptsNilDefinitionID(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-no-def-id")

	require.NoError(t, db.Create(mri).Error)
}

// TestMachineRuntimeInstance_BeforeCreate_RejectsMissingDefinition rejects an
// MRI whose definition id does not match a row.
func TestMachineRuntimeInstance_BeforeCreate_RejectsMissingDefinition(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)

	mri := newValidMRI("mri-missing-def")
	mri.MachineRuntimeDefinitionID = util.Ptr(uint(99999))

	err := db.Create(mri).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "which does not exist")
}

// TestMachineRuntimeInstance_BeforeCreate_ReturnsLookupFailure returns a
// database failure instead of the missing-definition message.
func TestMachineRuntimeInstance_BeforeCreate_ReturnsLookupFailure(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	mri := newValidMRI("mri-db-down")
	mri.MachineRuntimeDefinitionID = util.Ptr(uint(1))

	err = db.Create(mri).Error
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "which does not exist")
}

// TestMachineRuntimeInstance_BeforeUpdate_AllowsHostnameChange accepts an
// update of a mutable field on an imported MRI.
func TestMachineRuntimeInstance_BeforeUpdate_AllowsHostnameChange(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	mri := newValidMRI("mri-hostname")
	require.NoError(t, db.Create(mri).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)

	err := db.Model(&loaded).Updates(&MachineRuntimeInstance{
		Hostname: util.Ptr("other.example"),
	}).Error
	require.NoError(t, err)
}

// TestMachineRuntimeInstance_BeforeUpdate_RejectsAttachingProviderWithoutRegion
// rejects attaching a provider definition to an imported MRI that has no region.
func TestMachineRuntimeInstance_BeforeUpdate_RejectsAttachingProviderWithoutRegion(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	mri := newValidMRI("mri-attach-no-region")
	require.NoError(t, db.Create(mri).Error)

	mrd := &MachineRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr("mrd-attach-provider")},
		InfraProvider: util.Ptr("gce"),
	}
	require.NoError(t, db.Create(mrd).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)

	err := db.Model(&loaded).Updates(&MachineRuntimeInstance{
		MachineRuntimeDefinitionID: mrd.ID,
	}).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must have a region when the definition specifies an infra provider")
}

// TestMachineRuntimeInstance_BeforeUpdate_AcceptsAttachingProviderWithRegion
// accepts attaching a provider definition when the MRI already has a region.
func TestMachineRuntimeInstance_BeforeUpdate_AcceptsAttachingProviderWithRegion(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	mri := newValidMRI("mri-attach-region")
	mri.Region = util.Ptr("us-central1")
	require.NoError(t, db.Create(mri).Error)

	mrd := &MachineRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr("mrd-attach-provider-2")},
		InfraProvider: util.Ptr("gce"),
	}
	require.NoError(t, db.Create(mrd).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)

	err := db.Model(&loaded).Updates(&MachineRuntimeInstance{
		MachineRuntimeDefinitionID: mrd.ID,
	}).Error
	require.NoError(t, err)
}

// TestMachineRuntimeInstance_BeforeUpdate_RejectsSaveAttachingProviderWithoutRegion
// rejects a PUT that attaches a provider definition when the MRI has no region.
func TestMachineRuntimeInstance_BeforeUpdate_RejectsSaveAttachingProviderWithoutRegion(t *testing.T) {
	db := setupMachineWorkloadValidateDB(t)
	mri := newValidMRI("mri-save-attach-no-region")
	require.NoError(t, db.Create(mri).Error)

	mrd := &MachineRuntimeDefinition{
		Definition:    Definition{Name: util.Ptr("mrd-save-attach-provider")},
		InfraProvider: util.Ptr("gce"),
	}
	require.NoError(t, db.Create(mrd).Error)

	var loaded MachineRuntimeInstance
	require.NoError(t, db.First(&loaded, *mri.ID).Error)
	loaded.MachineRuntimeDefinitionID = mrd.ID

	err := db.Save(&loaded).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must have a region when the definition specifies an infra provider")
}
