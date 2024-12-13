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
