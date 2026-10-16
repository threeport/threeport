package v0

import "testing"

// TestEqual distinguishes a nil pointer from a pointer to the zero value.
func TestEqual(t *testing.T) {
	// a nil pointer does not match the zero value
	var missing *string
	if Equal(missing, "") {
		t.Fatal("nil pointer compared equal to the zero value")
	}
	// a pointer to kind matches kind
	kind := "kind"
	if !Equal(&kind, "kind") {
		t.Fatal("pointer to kind did not compare equal")
	}
	other := "eks"
	if Equal(&other, "kind") {
		t.Fatal("pointer to eks compared equal to kind")
	}
}
