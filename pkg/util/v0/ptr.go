package v0

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

// Deref returns the value of a pointer or the zero value of T if the
// pointer is nil. Mirror of Ptr.
//
// `var zero T` declares a variable of the generic type T initialized to
// Go's zero value for that type (nil for pointers/maps/slices/interfaces,
// 0 for numerics, "" for strings, the struct's zero literal for structs).
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// PtrEqual reports whether p is set and *p equals value.
// A nil pointer is not equal, including when value is the zero value.
func PtrEqual[T comparable](p *T, value T) bool {
	return p != nil && *p == value
}
