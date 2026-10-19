package v0

import (
	"reflect"
	"time"
)

// ReconciliationStateChanged reports whether progress markers differ.
// Acknowledgement timestamps count as set or unset only.
// InterruptReconciliation is ignored. It is a gate, not progress.
func ReconciliationStateChanged(a, b any) bool {
	return !boolPtrEqual(boolField(a, "Reconciled"), boolField(b, "Reconciled")) ||
		!timePtrSet(timeField(a, "CreationAcknowledged"), timeField(b, "CreationAcknowledged")) ||
		!timePtrEqual(timeField(a, "CreationConfirmed"), timeField(b, "CreationConfirmed")) ||
		!boolPtrEqual(boolField(a, "CreationFailed"), boolField(b, "CreationFailed")) ||
		!timePtrEqual(timeField(a, "DeletionScheduled"), timeField(b, "DeletionScheduled")) ||
		!timePtrSet(timeField(a, "DeletionAcknowledged"), timeField(b, "DeletionAcknowledged")) ||
		!timePtrEqual(timeField(a, "DeletionConfirmed"), timeField(b, "DeletionConfirmed"))
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
	// time.Time is copied by value so its unexported fields stay intact
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
// When Reconciled becomes true, the other changed fields are named and the write does not notify.
func ObjectUpdateNotifiable(before, after any) (bool, []string) {
	left, right, ok := persistedStructs(before, after)

	// notify when the values are nil, not structs, or different types.
	// The comparison cannot check the suppressions, and the row is already written.
	if !ok {
		return true, nil
	}

	// classify the write
	beforeState, beforeOK := reconciliationFrom(left)
	afterState, afterOK := reconciliationFrom(right)
	haveState := beforeOK && afterOK
	specChanged := structFieldsChanged(left, right, true)
	markersChanged := haveState && ReconciliationStateChanged(beforeState, afterState)
	restamped := haveState && acknowledgementRefreshed(beforeState, afterState)
	reconciledBecomesTrue := haveState && alreadyReconciled(afterState) && !alreadyReconciled(beforeState)

	// suppress the write that sets Reconciled true
	if reconciledBecomesTrue {
		return false, changedStructFields(left, right)
	}

	// suppress an acknowledgement restamp that changes nothing else
	if restamped && !specChanged && !markersChanged {
		return false, nil
	}

	// notify every remaining write, including equal snapshots.
	// Only Reconciled becoming true and an acknowledgement restamp skip the controller.
	return true, nil
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
		// recurse into the Common, Definition, Instance, and Reconciliation embeds.
		// Each contains no further embed, so the recursion stays one call deep.
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
		if skipReconciliation && field.Name == "Reconciliation" {
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
// time.Time uses Equal so a database round trip does not count as a change.
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
	// reflect.DeepEqual covers slices, maps, and plain values
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
func reconciliationFrom(object reflect.Value) (any, bool) {
	// read the embedded struct named Reconciliation
	field := object.FieldByName("Reconciliation")
	if !field.IsValid() || field.Kind() != reflect.Struct {
		return nil, false
	}
	return field.Interface(), true
}

// acknowledgementRefreshed reports a restamped creation or deletion acknowledgement.
func acknowledgementRefreshed(a, b any) bool {
	return timePtrRefreshed(timeField(a, "CreationAcknowledged"), timeField(b, "CreationAcknowledged")) ||
		timePtrRefreshed(timeField(a, "DeletionAcknowledged"), timeField(b, "DeletionAcknowledged"))
}

// alreadyReconciled reports whether Reconciled is set to true.
func alreadyReconciled(state any) bool {
	// treat a nil pointer as not reconciled
	reconciled := boolField(state, "Reconciled")
	return reconciled != nil && *reconciled
}

// fieldByName returns an exported field of a struct or a pointer to one.
func fieldByName(state any, name string) reflect.Value {
	value := reflect.ValueOf(state)
	if !value.IsValid() {
		return reflect.Value{}
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return value.FieldByName(name)
}

// boolField returns a copy of a *bool field. A missing field counts as unset.
func boolField(state any, name string) *bool {
	field := fieldByName(state, name)
	if !field.IsValid() || field.Kind() != reflect.Pointer || field.IsNil() {
		return nil
	}
	value := field.Elem().Bool()
	return &value
}

// timeField returns a copy of a *time.Time field. A missing field counts as unset.
func timeField(state any, name string) *time.Time {
	field := fieldByName(state, name)
	if !field.IsValid() || field.Kind() != reflect.Pointer || field.IsNil() {
		return nil
	}
	value := field.Elem().Interface().(time.Time)
	return &value
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

// timePtrEqual compares instants.
// == on time.Time also compares location and the monotonic reading, which a database round trip drops.
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// timePtrSet reports whether a and b are both set or both unset.
func timePtrSet(a, b *time.Time) bool {
	// compare presence, not the instant
	return (a == nil) == (b == nil)
}
