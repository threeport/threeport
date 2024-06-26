package v0

import (
	"time"

	"gorm.io/datatypes"
)

// IntPtr returns a pointer to the int value passed in.
func IntPtr(i int) *int {
	return &i
}

// BoolPtr returns a pointer to the bool value passed in.
func BoolPtr(b bool) *bool {
	return &b
}

// TimePtr returns a pointer to the time value passed in.
func TimePtr(t time.Time) *time.Time {
	return &t
}

// StringPtr returns a pointer to the string value passed in.
func StringPtr(s string) *string {
	return &s
}

// JsonPtr returns a pointer to the datatypes.JSON value passed in.
func JsonPtr(j datatypes.JSON) *datatypes.JSON {
	return &j
}

// Ptr returns a pointer to the value passed in.
func Ptr[T any](input T) *T {
	return &input
}

// DerefString returns the value of a string pointer or an empty
// string if the pointer is nil.
func DerefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
