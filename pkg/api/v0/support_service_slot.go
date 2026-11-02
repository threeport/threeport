package v0

import (
	"fmt"
	"reflect"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// supportServiceSlot is the tables and fields a shared workload column compares.
// The runtime update builds one value for its own columns.
type supportServiceSlot struct {
	fields         []string
	runtimeTable   string
	workloadTable  string
	referenceTable string
	workloadType   string
	relationship   string
}

// applySupportServiceSlots claims or clears a shared workload slot inside the update transaction.
func applySupportServiceSlots(tx *gorm.DB, runtimeID uint) error {
	// name the runtime columns and the tables this update compares
	slot := supportServiceSlot{
		fields: []string{
			"IngressComponentInstanceID",
			"CertificatesComponentInstanceID",
			"SecretsComponentInstanceID",
			"DatabaseComponentInstanceID",
		},
		runtimeTable:   KubernetesRuntimeInstance{}.TableName(),
		workloadTable:  KubernetesWorkloadInstance{}.TableName(),
		referenceTable: AttachedObjectReference{}.TableName(),
		workloadType:   (&KubernetesWorkloadInstance{}).GetFullyQualifiedType(),
		relationship:   string(RelationshipRequires),
	}

	// skip an update that carries no runtime
	incoming := lib.IncomingValues(tx)
	if len(slot.fields) == 0 || !hasSlotField(incoming, slot.fields[0]) {
		return nil
	}

	// load the stored row once when a clear needs the id the payload leaves out
	var stored any
	for _, field := range slot.fields {
		changed, err := lib.IsFieldChanged(tx, field)
		if err != nil {
			return fmt.Errorf("failed to check %s on kubernetes runtime instance %d: %w", field, runtimeID, err)
		}
		if !changed {
			continue
		}
		column, err := slotColumn(tx, field)
		if err != nil {
			return err
		}
		next := slotValue(incoming, field)
		if next != nil {
			if err := claimSupportServiceSlot(tx, slot, column, runtimeID, *next); err != nil {
				return err
			}
			continue
		}
		if stored == nil {
			loaded, err := lib.LoadObjectFromDB(tx)
			if err != nil {
				return fmt.Errorf("failed to load kubernetes runtime instance %d: %w", runtimeID, err)
			}
			stored = loaded
		}
		previous := slotValue(stored, field)
		if previous == nil {
			continue
		}
		cleared, err := clearSupportServiceSlot(tx, slot, column, runtimeID, *previous)
		if err != nil {
			return err
		}
		if !cleared {
			return util.NewBadRequestError(lib.ErrMsgSupportServiceSlotHeld)
		}
	}
	return nil
}

// claimSupportServiceSlot stores the workload when the slot is empty or already matches.
// It also stores the workload when the named row is gone.
func claimSupportServiceSlot(tx *gorm.DB, slot supportServiceSlot, column string, runtimeID, workloadID uint) error {
	// claim the slot when it is empty, matches, or names a deleted workload
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
		slot.runtimeTable,
		column,
		column,
		column,
		slot.workloadTable,
		slot.runtimeTable,
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
		return util.NewBadRequestError(lib.ErrMsgSupportServiceSlotHeld)
	}
	return nil
}

// clearSupportServiceSlot clears the slot when nothing still requires its workload.
func clearSupportServiceSlot(tx *gorm.DB, slot supportServiceSlot, column string, runtimeID, workloadID uint) (bool, error) {
	// clear the column when nothing requires the workload
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
		slot.runtimeTable,
		column,
		column,
		slot.referenceTable,
	)
	result := lib.NewCleanSession(tx).Exec(
		statement,
		runtimeID,
		workloadID,
		slot.workloadType,
		workloadID,
		slot.relationship,
	)
	if result.Error != nil {
		return false, fmt.Errorf("failed to clear %s on kubernetes runtime instance %d: %w", column, runtimeID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// slotColumn returns the database column for a slot field.
func slotColumn(tx *gorm.DB, field string) (string, error) {
	// read the column the field stores
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

// hasSlotField reports whether object is a struct pointer with field.
func hasSlotField(object any, field string) bool {
	// reject a value that is not the runtime struct
	value := reflect.ValueOf(object)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return false
	}
	return value.Elem().FieldByName(field).IsValid()
}

// slotValue reads one *uint slot field by name.
func slotValue(object any, field string) *uint {
	// return the pointer for the named slot
	value := reflect.ValueOf(object)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return nil
	}
	slot := value.Elem().FieldByName(field)
	if !slot.IsValid() || slot.Kind() != reflect.Pointer || slot.IsNil() {
		return nil
	}
	id, ok := slot.Interface().(*uint)
	if !ok {
		return nil
	}
	return id
}
