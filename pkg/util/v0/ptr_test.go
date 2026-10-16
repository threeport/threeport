package v0

import "testing"

// TestPtrEqual distinguishes a nil pointer from a pointer to the zero value.
func TestPtrEqual(t *testing.T) {
	// a nil pointer does not match the zero value
	var missing *string
	if PtrEqual(missing, "") {
		t.Fatal("nil pointer compared equal to the zero value")
	}
	// a pointer to kind matches kind
	kind := "kind"
	if !PtrEqual(&kind, "kind") {
		t.Fatal("pointer to kind did not compare equal")
	}
	other := "eks"
	if PtrEqual(&other, "kind") {
		t.Fatal("pointer to eks compared equal to kind")
	}
}
