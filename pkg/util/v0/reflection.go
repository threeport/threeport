package v0

import (
	"errors"
	"reflect"
)

// GetStringPtrValue returns the string value of a pointer field.
func GetStringPtrValue(field reflect.Value) (string, error) {
	if field.Kind() != reflect.Ptr {
		return "", errors.New("field value is not a pointer")
	}
	if field.IsNil() {
		return "", errors.New("field value is nil")
	}
	createdVal, ok := field.Elem().Interface().(string)
	if !ok {
		return "", errors.New("field value is not a string")
	}
	return createdVal, nil
}
