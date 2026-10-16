package v0

import (
	"reflect"
	"time"

	"gorm.io/gorm"
)

// Common includes standard fields included in most objects.
type Common struct {
	ID        *uint `gorm:"primarykey"`
	CreatedAt *time.Time
	UpdatedAt *time.Time
	DeletedAt *gorm.DeletedAt `gorm:"index"`
}

// Reconciliation includes the fields for reconciled objects.  These are
// leveraged by controllers to persist information related to the reconciliation
// of system state for objects.
type Reconciliation struct {
	// Indicates if object is considered to be reconciled by the object's controller.
	Reconciled *bool `validate:"optional" gorm:"default:false"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun so that subsequent reconciliation attempts can
	// act accordingly.
	CreationAcknowledged *time.Time `validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	CreationConfirmed *time.Time `validate:"optional"`

	// Gets set to true if creation process fails.
	CreationFailed *bool `validate:"optional" gorm:"default:false"`

	// Used to inform reconcilers that an object is being deleted so they may
	// complete delete reconciliation before actually deleting the object from the database.
	DeletionScheduled *time.Time `validate:"optional"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun so that subsequent reconciliation attempts can
	// act accordingly.
	DeletionAcknowledged *time.Time `validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	DeletionConfirmed *time.Time `validate:"optional"`

	// InterruptReconciliation is used by the controller to indicated that future
	// reconcilation should be interrupted.  Useful in cases where there is a
	// situation where future reconciliation could be descructive such as
	// spinning up more infrastructure when there is a unresolved problem.
	InterruptReconciliation *bool `validate:"optional" gorm:"default:false"`
}

// AlreadyReconciled reports whether Reconciled is set to true.
func (r Reconciliation) AlreadyReconciled() bool {
	return r.Reconciled != nil && *r.Reconciled
}

// ReconciliationStateChanged reports whether progress markers differ.
// Acknowledgement timestamps count as set or unset only. InterruptReconciliation
// is ignored; it is a gate, not progress.
func ReconciliationStateChanged(a, b Reconciliation) bool {
	return !boolPtrEqual(a.Reconciled, b.Reconciled) ||
		!timePtrSet(a.CreationAcknowledged, b.CreationAcknowledged) ||
		!timePtrEqual(a.CreationConfirmed, b.CreationConfirmed) ||
		!boolPtrEqual(a.CreationFailed, b.CreationFailed) ||
		!timePtrEqual(a.DeletionScheduled, b.DeletionScheduled) ||
		!timePtrSet(a.DeletionAcknowledged, b.DeletionAcknowledged) ||
		!timePtrEqual(a.DeletionConfirmed, b.DeletionConfirmed)
}

// NotifySnapshot returns a copy of a saved object.
// GORM writes new column values through the object's existing pointers, so the copy must own its own.
func NotifySnapshot[T any](object T) T {
	copied := deepCopy(reflect.ValueOf(object))
	value, ok := copied.Interface().(T)
	if !ok {
		return object
	}
	return value
}

// deepCopy returns a copy that does not share pointers with value.
func deepCopy(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.Type() == reflect.TypeOf(time.Time{}) {
		return value
	}

	// copy each kind that can share a pointer
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copied := reflect.New(value.Type().Elem())
		copied.Elem().Set(deepCopy(value.Elem()))
		return copied
	case reflect.Struct:
		copied := reflect.New(value.Type()).Elem()
		for index := 0; index < value.NumField(); index++ {
			if !value.Type().Field(index).IsExported() {
				continue
			}
			copied.Field(index).Set(deepCopy(value.Field(index)))
		}
		return copied
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copied := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			copied.Index(index).Set(deepCopy(value.Index(index)))
		}
		return copied
	case reflect.Array:
		copied := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			copied.Index(index).Set(deepCopy(value.Index(index)))
		}
		return copied
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copied := reflect.MakeMapWithSize(value.Type(), value.Len())
		for _, key := range value.MapKeys() {
			copied.SetMapIndex(deepCopy(key), deepCopy(value.MapIndex(key)))
		}
		return copied
	default:
		return value
	}
}

// ObjectUpdateNotifiable reports whether a saved object should notify the controller.
// changed names the other persisted fields when Reconciled becomes true, which does not notify.
func ObjectUpdateNotifiable(before, after any) (bool, []string) {
	left, right, ok := persistedStructs(before, after)
	if !ok {
		return true, nil
	}

	// honor a client write that sets Reconciled true
	beforeState, beforeOK := reconciliationFrom(left)
	afterState, afterOK := reconciliationFrom(right)
	if beforeOK && afterOK && afterState.AlreadyReconciled() && !beforeState.AlreadyReconciled() {
		return false, changedPersistedFields(left, right)
	}

	// notify when a field outside reconciliation state changed
	if persistedFieldsChanged(left, right) {
		return true, nil
	}
	if !beforeOK || !afterOK {
		return true, nil
	}
	return ReconciliationUpdateNotifiable(beforeState, afterState), nil
}

// changedPersistedFields names persisted fields that differ.
// UpdatedAt and Reconciled are omitted.
func changedPersistedFields(left, right reflect.Value) []string {
	return changedStructFields(left, right)
}

// changedStructFields names exported fields that differ, descending into embeds.
func changedStructFields(left, right reflect.Value) []string {
	// name each exported field that differs, except UpdatedAt and Reconciled
	var names []string
	objectType := left.Type()
	for index := 0; index < objectType.NumField(); index++ {
		field := objectType.Field(index)
		if !field.IsExported() || field.Name == "UpdatedAt" || field.Name == "Reconciled" {
			continue
		}
		leftField := left.Field(index)
		rightField := right.Field(index)
		if field.Anonymous && leftField.Kind() == reflect.Struct {
			names = append(names, changedStructFields(leftField, rightField)...)
			continue
		}
		if !fieldValuesEqual(leftField, rightField) {
			names = append(names, field.Name)
		}
	}
	return names
}

// persistedFieldsChanged reports a saved-field change outside reconciliation state.
// UpdatedAt moves on every write, so it is not one of those fields.
func persistedFieldsChanged(left, right reflect.Value) bool {
	return structFieldsChanged(left, right, true)
}

// structFieldsChanged reports an exported-field change.
// skipReconciliation drops the embedded reconciliation state.
func structFieldsChanged(left, right reflect.Value, skipReconciliation bool) bool {
	// skip reconciliation state and UpdatedAt, then compare the other fields
	objectType := left.Type()
	for index := 0; index < objectType.NumField(); index++ {
		field := objectType.Field(index)
		if !field.IsExported() {
			continue
		}
		if skipReconciliation && field.Type == reflect.TypeOf(Reconciliation{}) {
			continue
		}
		if field.Name == "UpdatedAt" {
			continue
		}
		if !fieldValuesEqual(left.Field(index), right.Field(index)) {
			return true
		}
	}
	return false
}

// fieldValuesEqual reports whether two saved field values match.
func fieldValuesEqual(left, right reflect.Value) bool {
	if !left.IsValid() || !right.IsValid() {
		return left.IsValid() == right.IsValid()
	}
	if left.Type() != right.Type() {
		return false
	}

	// compare instants without the monotonic clock or the location
	if left.Type() == reflect.TypeOf(time.Time{}) {
		return left.Interface().(time.Time).Equal(right.Interface().(time.Time))
	}
	if left.Type() == reflect.TypeOf((*time.Time)(nil)) {
		return timePtrEqual(left.Interface().(*time.Time), right.Interface().(*time.Time))
	}

	// compare pointers and structs by their fields
	switch left.Kind() {
	case reflect.Pointer:
		if left.IsNil() || right.IsNil() {
			return left.IsNil() == right.IsNil()
		}
		return fieldValuesEqual(left.Elem(), right.Elem())
	case reflect.Struct:
		return !structFieldsChanged(left, right, false)
	default:
		return reflect.DeepEqual(left.Interface(), right.Interface())
	}
}

// persistedStructs returns the struct values of the two saved objects.
func persistedStructs(before, after any) (reflect.Value, reflect.Value, bool) {
	left, leftOK := persistedStruct(before)
	right, rightOK := persistedStruct(after)
	if !leftOK || !rightOK || left.Type() != right.Type() {
		return reflect.Value{}, reflect.Value{}, false
	}
	return left, right, true
}

// persistedStruct returns the struct a saved object value points at.
func persistedStruct(object any) (reflect.Value, bool) {
	value := reflect.ValueOf(object)
	if !value.IsValid() {
		return reflect.Value{}, false
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	return value, true
}

// reconciliationFrom returns the embedded reconciliation state.
func reconciliationFrom(object reflect.Value) (Reconciliation, bool) {
	field := object.FieldByName("Reconciliation")
	if !field.IsValid() || field.Type() != reflect.TypeOf(Reconciliation{}) {
		return Reconciliation{}, false
	}
	state, ok := field.Interface().(Reconciliation)
	return state, ok
}

// ReconciliationUpdateNotifiable reports whether an update should notify the
// controller. Spec edits notify even when markers are unchanged. A restamped
// acknowledgement does not, and neither does the controller's own stamp of
// Reconciled to true.
func ReconciliationUpdateNotifiable(a, b Reconciliation) bool {
	if reconciledStampOnly(a, b) {
		return false
	}
	if ReconciliationStateChanged(a, b) {
		return true
	}
	return !acknowledgementRefreshed(a, b)
}

// reconciledStampOnly reports a write whose only marker change is Reconciled
// becoming true.
func reconciledStampOnly(a, b Reconciliation) bool {
	if !b.AlreadyReconciled() || a.AlreadyReconciled() {
		return false
	}

	// compare the other markers with Reconciled held equal
	same := a
	same.Reconciled = b.Reconciled
	return !ReconciliationStateChanged(same, b)
}

// acknowledgementRefreshed reports a restamped creation or deletion acknowledgement.
func acknowledgementRefreshed(a, b Reconciliation) bool {
	return timePtrRefreshed(a.CreationAcknowledged, b.CreationAcknowledged) ||
		timePtrRefreshed(a.DeletionAcknowledged, b.DeletionAcknowledged)
}

// timePtrRefreshed reports two set pointers holding different instants.
func timePtrRefreshed(a, b *time.Time) bool {
	return a != nil && b != nil && !a.Equal(*b)
}

// boolPtrEqual reports whether a and b are both unset or hold the same value.
func boolPtrEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// timePtrEqual compares instants. == on time.Time also compares location and
// the monotonic reading, which a database round trip drops.
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// timePtrSet reports whether a and b are both set or both unset.
func timePtrSet(a, b *time.Time) bool {
	return (a == nil) == (b == nil)
}
