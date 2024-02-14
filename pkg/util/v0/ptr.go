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
