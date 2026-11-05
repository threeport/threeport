package v0

import (
	"fmt"
	"reflect"

	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// ApplySupportServiceSlots claims or clears a shared workload slot inside the update transaction.
// The function lives in this package, and takes the API names as arguments, to keep both packages clean.
func ApplySupportServiceSlots(
	tx *gorm.DB,
	runtimeID uint,
	model any,
	fields []string,
	runtimeTable, workloadTable, referenceTable, workloadType, relationship, heldMessage string,
) error {
	// skip an update that is not this runtime
	incoming := IncomingValues(tx)
	if len(fields) == 0 || !hasSlotField(incoming, fields[0]) {
		return nil
	}

	// claim or clear each changed slot
	var stored any
	for _, field := range fields {
		// leave a field the payload does not change
		changed, err := IsFieldChanged(tx, field)
		if err != nil {
			return fmt.Errorf("failed to check %s on kubernetes runtime instance %d: %w", field, runtimeID, err)
		}
		if !changed {
			continue
		}

		// map the field to its column
		column, err := slotColumn(tx, model, field)
		if err != nil {
			return err
		}

		// claim the workload id the payload carries
		next := slotValue(incoming, field)
		if next != nil {
			if err := claimSupportServiceSlot(tx, runtimeTable, workloadTable, column, runtimeID, *next, heldMessage); err != nil {
				return err
			}
			continue
		}

		// load the stored row once for the id a clear leaves out
		if stored == nil {
			loaded, err := LoadObjectFromDB(tx)
			if err != nil {
				return fmt.Errorf("failed to load kubernetes runtime instance %d: %w", runtimeID, err)
			}
			stored = loaded
		}

		// skip a slot that was already empty
		previous := slotValue(stored, field)
		if previous == nil {
			continue
		}

		// clear the slot, or reject the update when it is still required
		cleared, err := clearSupportServiceSlot(tx, runtimeTable, referenceTable, workloadType, relationship, column, runtimeID, *previous)
		if err != nil {
			return err
		}
		if !cleared {
			return util.NewBadRequestError(heldMessage)
		}
	}
	return nil
}

// claimSupportServiceSlot stores the workload when the slot is empty or already matches.
// It also stores the workload when the named row is gone.
func claimSupportServiceSlot(
	tx *gorm.DB,
	runtimeTable, workloadTable, column string,
	runtimeID, workloadID uint,
	heldMessage string,
) error {
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
		runtimeTable,
		column,
		column,
		column,
		workloadTable,
		runtimeTable,
		column,
	)
	result := NewCleanSession(tx).Exec(
		statement,
		workloadID,
		runtimeID,
		workloadID,
	)
	if result.Error != nil {
		return fmt.Errorf("failed to claim %s on kubernetes runtime instance %d: %w", column, runtimeID, result.Error)
	}
	if result.RowsAffected != 1 {
		return util.NewBadRequestError(heldMessage)
	}
	return nil
}

// clearSupportServiceSlot clears the slot when nothing still requires its workload.
func clearSupportServiceSlot(
	tx *gorm.DB,
	runtimeTable, referenceTable, workloadType, relationship, column string,
	runtimeID, workloadID uint,
) (bool, error) {
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
		runtimeTable,
		column,
		column,
		referenceTable,
	)
	result := NewCleanSession(tx).Exec(
		statement,
		runtimeID,
		workloadID,
		workloadType,
		workloadID,
		relationship,
	)
	if result.Error != nil {
		return false, fmt.Errorf("failed to clear %s on kubernetes runtime instance %d: %w", column, runtimeID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// slotColumn returns the database column for a slot field.
func slotColumn(tx *gorm.DB, model any, field string) (string, error) {
	// read the column the field stores
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(model); err != nil {
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
