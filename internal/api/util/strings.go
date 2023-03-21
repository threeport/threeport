package util

import "strings"

// Contains returns true if an array contains certain string
func Contains(sl []string, name string, caseSensitive bool) bool {
	for _, value := range sl {
		switch caseSensitive {
		case true:
			if value == name {
				return true
			}
		case false:
			if strings.EqualFold(value, name) {
				return true
			}
		}
	}
	return false
}

// StringToPointer converts a string to a pointer
func StringToPointer(in string) *string {
	return &in
}

// BoolToPointer converts a bool to a pointer
func BoolToPointer(in bool) *bool {
	return &in
}

// Int32ToPointer  converts a int32 to a pointer
func Int32ToPointer(in int32) *int32 {
	i := int32(in)
	return &i
}
