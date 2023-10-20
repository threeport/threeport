package v0

import (
	"errors"
	"reflect"
)

// GetStringPtrValue returns the string value of a pointer field.
func GetStringPtrValue(field reflect.Value) (string, error) {
	if !IsNonNilPtr(field) {
		return "", nil
	}
	createdVal, ok := field.Elem().Interface().(string)
	if !ok {
		return "", errors.New("field value is not a string")
	}
	return createdVal, nil
}

// IsNilPtr returns true if the field is a nil pointer.
func IsNonNilPtr(field reflect.Value) bool {
	if field.Kind() != reflect.Ptr {
		return false
	}
	return field.IsNil() == false
}