package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// openSlotDB returns a database with the runtime and workload tables.
func openSlotDB(t *testing.T) *gorm.DB {
	t.Helper()

	// open the runtime database and add the workload tables
	db := setupKubernetesRuntimeValidateDB(t)
	require.NoError(t, db.AutoMigrate(&KubernetesWorkloadDefinition{}, &KubernetesWorkloadInstance{}))
	return db
}

// claimSlot updates the runtime slot to workloadID.
func claimSlot(t *testing.T, db *gorm.DB, runtimeID uint, workloadID uint) error {
	t.Helper()

	// write the slot on the stored row
	var loaded KubernetesRuntimeInstance
	if err := db.First(&loaded, runtimeID).Error; err != nil {
		return err
	}
	return db.Model(&loaded).
		Updates(&KubernetesRuntimeInstance{GatewayControllerInstanceID: &workloadID}).
		Error
}

// storedSlot reads the controller slot back from the database.
func storedSlot(t *testing.T, db *gorm.DB, runtimeID uint) *uint {
	t.Helper()

	// read the runtime
	var stored KubernetesRuntimeInstance
	require.NoError(t, db.First(&stored, runtimeID).Error)
	return stored.GatewayControllerInstanceID
}

// TestSupportServiceSlotClaimsTheDatabaseColumn stores an id on the database column.
func TestSupportServiceSlotClaimsTheDatabaseColumn(t *testing.T) {
	db := openSlotDB(t)
	runtime := createValidKRI(t, db, "Local")

	// claim the database column with no workload row
	var loaded KubernetesRuntimeInstance
	require.NoError(t, db.First(&loaded, *runtime.ID).Error)
	loaded.DatabaseControllerInstanceID = util.Ptr(uint(11))
	require.NoError(t, db.Model(&loaded).Updates(&loaded).Error)

	var stored KubernetesRuntimeInstance
	require.NoError(t, db.First(&stored, *runtime.ID).Error)
	require.NotNil(t, stored.DatabaseControllerInstanceID)
	require.Equal(t, uint(11), *stored.DatabaseControllerInstanceID)
}

// TestSupportServiceSlotClaimsAnEmptyColumn stores the first workload id.
func TestSupportServiceSlotClaimsAnEmptyColumn(t *testing.T) {
	// arrange a runtime with an empty slot
	db := openSlotDB(t)
	runtime := createValidKRI(t, db, "Local")

	// claim
	require.NoError(t, claimSlot(t, db, *runtime.ID, 7))

	// the slot holds that id
	got := storedSlot(t, db, *runtime.ID)
	require.NotNil(t, got)
	require.Equal(t, uint(7), *got)
}

// TestSupportServiceSlotRejectsADifferentLiveWorkload keeps the recorded id.
func TestSupportServiceSlotRejectsADifferentLiveWorkload(t *testing.T) {
	// arrange a slot that names a live workload
	db := openSlotDB(t)
	runtime := createValidKRI(t, db, "Local")
	workload := createSlotWorkload(t, db, &runtime, "held-workload")
	require.NoError(t, claimSlot(t, db, *runtime.ID, *workload.ID))

	// claim a different id
	err := claimSlot(t, db, *runtime.ID, *workload.ID+1)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), ErrMsgSupportServiceSlotHeld))

	// the slot stays on the live workload
	got := storedSlot(t, db, *runtime.ID)
	require.NotNil(t, got)
	require.Equal(t, *workload.ID, *got)
}

// TestSupportServiceSlotReplacesADeletedWorkload accepts a new id.
func TestSupportServiceSlotReplacesADeletedWorkload(t *testing.T) {
	// arrange a slot whose workload row is gone
	db := openSlotDB(t)
	runtime := createValidKRI(t, db, "Local")
	workload := createSlotWorkload(t, db, &runtime, "gone-workload")
	require.NoError(t, claimSlot(t, db, *runtime.ID, *workload.ID))
	require.NoError(t, db.Delete(workload).Error)
	replacement := createSlotWorkload(t, db, &runtime, "replacement")

	// claim the replacement
	require.NoError(t, claimSlot(t, db, *runtime.ID, *replacement.ID))

	// the slot holds the replacement
	got := storedSlot(t, db, *runtime.ID)
	require.NotNil(t, got)
	require.Equal(t, *replacement.ID, *got)
}

// TestSupportServiceSlotRejectsClearWhileRequired keeps the slot.
func TestSupportServiceSlotRejectsClearWhileRequired(t *testing.T) {
	// arrange a live requires row for the recorded workload
	db := openSlotDB(t)
	runtime := createValidKRI(t, db, "Local")
	workload := createSlotWorkload(t, db, &runtime, "required-workload")
	require.NoError(t, claimSlot(t, db, *runtime.ID, *workload.ID))
	require.NoError(t, db.Create(requiresWorkload(workload)).Error)

	// clear the slot
	err := clearSlot(t, db, *runtime.ID)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), ErrMsgSupportServiceSlotHeld))

	// the slot stays set
	got := storedSlot(t, db, *runtime.ID)
	require.NotNil(t, got)
	require.Equal(t, *workload.ID, *got)
}

// TestSupportServiceSlotClearsWhenTheRequirementIsDeleted nils the slot.
func TestSupportServiceSlotClearsWhenTheRequirementIsDeleted(t *testing.T) {
	// arrange a requires row that has been deleted
	db := openSlotDB(t)
	runtime := createValidKRI(t, db, "Local")
	workload := createSlotWorkload(t, db, &runtime, "released-workload")
	require.NoError(t, claimSlot(t, db, *runtime.ID, *workload.ID))
	reference := requiresWorkload(workload)
	require.NoError(t, db.Create(reference).Error)
	require.NoError(t, db.Delete(reference).Error)

	// clear the slot
	require.NoError(t, clearSlot(t, db, *runtime.ID))

	// the slot is empty
	require.Nil(t, storedSlot(t, db, *runtime.ID))
}

// createSlotWorkload inserts a workload definition and instance.
func createSlotWorkload(
	t *testing.T,
	db *gorm.DB,
	runtime *KubernetesRuntimeInstance,
	name string,
) *KubernetesWorkloadInstance {
	t.Helper()

	// create the definition and the instance
	definition := &KubernetesWorkloadDefinition{
		Definition:   Definition{Name: util.Ptr(name + "-definition")},
		YAMLDocument: util.Ptr("kind: List"),
	}
	require.NoError(t, db.Create(definition).Error)
	workload := &KubernetesWorkloadInstance{
		Instance:                       Instance{Name: util.Ptr(name)},
		KubernetesRuntimeInstanceID:    runtime.ID,
		KubernetesWorkloadDefinitionID: definition.ID,
	}
	require.NoError(t, db.Create(workload).Error)
	return workload
}

// requiresWorkload returns a live requires row for the workload.
func requiresWorkload(workload *KubernetesWorkloadInstance) *AttachedObjectReference {
	// point the row at the workload
	relationship := RelationshipRequires
	return &AttachedObjectReference{
		ObjectType:         util.Ptr(workload.GetFullyQualifiedType()),
		ObjectID:           workload.ID,
		AttachedObjectType: util.Ptr("threeport.io/v0.GatewayInstance"),
		AttachedObjectID:   util.Ptr(uint(1)),
		Relationship:       &relationship,
	}
}

// clearSlot saves the runtime with an empty controller slot.
func clearSlot(t *testing.T, db *gorm.DB, runtimeID uint) error {
	t.Helper()

	// save the runtime with the slot empty
	var loaded KubernetesRuntimeInstance
	require.NoError(t, db.First(&loaded, runtimeID).Error)
	loaded.GatewayControllerInstanceID = nil
	return db.Save(&loaded).Error
}
