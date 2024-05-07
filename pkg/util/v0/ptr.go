package v0

// Ptr returns a pointer to the value passed in.
func Ptr[T any](input T) *T {
	return &input
}
