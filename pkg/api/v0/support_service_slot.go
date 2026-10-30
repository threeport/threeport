package v0

import (
	"fmt"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ErrMsgSupportServiceSlotHeld is the API message when a slot write loses the compare.
const ErrMsgSupportServiceSlotHeld = "support service slot is held"

// supportServiceSlotFields are the runtime columns that store one shared workload id.
var supportServiceSlotFields = []string{
	"GatewayControllerInstanceID",
	"DnsControllerInstanceID",
	"SecretsControllerInstanceID",
}

// applySupportServiceSlots claims or clears a shared workload slot inside the update transaction.
func (k *KubernetesRuntimeInstance) applySupportServiceSlots(tx *gorm.DB) error {
	incoming, ok := lib.IncomingValues(tx).(*KubernetesRuntimeInstance)
	if !ok || incoming == nil {
		return nil
	}

	// load the stored row once when a clear needs the id the payload leaves out
	var stored *KubernetesRuntimeInstance
	for _, field := range supportServiceSlotFields {
		changed, err := lib.IsFieldChanged(tx, field)
		if err != nil {
			return fmt.Errorf("failed to check %s on kubernetes runtime instance %d: %w", field, *k.ID, err)
		}
		if !changed {
			continue
		}
		column, err := supportServiceColumn(tx, field)
		if err != nil {
			return err
		}
		next := supportServiceSlotValue(incoming, field)
		if next != nil {
			if err := claimSupportServiceSlot(tx, column, *k.ID, *next); err != nil {
				return err
			}
			continue
		}
		if stored == nil {
			loaded, err := lib.LoadObjectFromDB(tx)
			if err != nil {
				return fmt.Errorf("failed to load kubernetes runtime instance %d: %w", *k.ID, err)
			}
			stored = loaded.(*KubernetesRuntimeInstance)
		}
		previous := supportServiceSlotValue(stored, field)
		if previous == nil {
			continue
		}
		cleared, err := clearSupportServiceSlot(tx, column, *k.ID, *previous)
		if err != nil {
			return err
		}
		if !cleared {
			return util.NewBadRequestError(ErrMsgSupportServiceSlotHeld)
		}
	}
	return nil
}

// claimSupportServiceSlot writes the slot when the column is empty, already holds this workload, or points at a deleted workload row.
func claimSupportServiceSlot(tx *gorm.DB, column string, runtimeID uint, workloadID uint) error {
	runtimeTable := (&KubernetesRuntimeInstance{}).TableName()
	workloadTable := (&KubernetesWorkloadInstance{}).TableName()
	statement := fmt.Sprintf(`
		UPDATE %s
		SET %s = ?
		WHERE id = ?
		  AND deleted_at IS NULL
		  AND (
		    %s IS NULL
		    OR %s = ?
		    OR NOT EXISTS (
		      SELECT 1 FROM %s AS w
		      WHERE w.id = %s.%s
		        AND w.deleted_at IS NULL
		    )
		  )`,
		runtimeTable,
		column,
		column,
		column,
		workloadTable,
		runtimeTable,
		column,
	)
	result := lib.NewCleanSession(tx).Exec(
		statement,
		workloadID,
		runtimeID,
		workloadID,
	)
	if result.Error != nil {
		return fmt.Errorf("failed to claim %s on kubernetes runtime instance %d: %w", column, runtimeID, result.Error)
	}
	if result.RowsAffected != 1 {
		return util.NewBadRequestError(ErrMsgSupportServiceSlotHeld)
	}
	return nil
}

// clearSupportServiceSlot nils the slot when no live requires row points at its workload.
func clearSupportServiceSlot(tx *gorm.DB, column string, runtimeID uint, workloadID uint) (bool, error) {
	runtimeTable := (&KubernetesRuntimeInstance{}).TableName()
	referenceTable := (&AttachedObjectReference{}).TableName()
	workloadType := (&KubernetesWorkloadInstance{}).GetFullyQualifiedType()
	statement := fmt.Sprintf(`
		UPDATE %s
		SET %s = NULL
		WHERE id = ?
		  AND %s = ?
		  AND deleted_at IS NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM %s AS a
		    WHERE a.object_type = ?
		      AND a.object_id = ?
		      AND a.relationship = ?
		      AND a.deleted_at IS NULL
		  )`,
		runtimeTable,
		column,
		column,
		referenceTable,
	)
	result := lib.NewCleanSession(tx).Exec(
		statement,
		runtimeID,
		workloadID,
		workloadType,
		workloadID,
		RelationshipRequires,
	)
	if result.Error != nil {
		return false, fmt.Errorf("failed to clear %s on kubernetes runtime instance %d: %w", column, runtimeID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// supportServiceColumn returns the database column for a runtime slot field.
func supportServiceColumn(tx *gorm.DB, field string) (string, error) {
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(&KubernetesRuntimeInstance{}); err != nil {
		return "", fmt.Errorf("failed to parse kubernetes runtime instance schema: %w", err)
	}
	parsed := statement.Schema.LookUpField(field)
	if parsed == nil {
		return "", fmt.Errorf("kubernetes runtime instance has no field %s", field)
	}
	return parsed.DBName, nil
}

// supportServiceSlotValue reads one slot field from a runtime.
func supportServiceSlotValue(runtime *KubernetesRuntimeInstance, field string) *uint {
	// return the pointer for the named slot
	switch field {
	case "GatewayControllerInstanceID":
		return runtime.GatewayControllerInstanceID
	case "DnsControllerInstanceID":
		return runtime.DnsControllerInstanceID
	case "SecretsControllerInstanceID":
		return runtime.SecretsControllerInstanceID
	default:
		return nil
	}
}
