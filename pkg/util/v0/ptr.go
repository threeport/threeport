package v0

import "time"

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
