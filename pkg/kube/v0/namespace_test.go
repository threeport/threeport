package v0

import (
	"strconv"
	"strings"
	"testing"
)

// TestManagedNamespaceNameStable checks that one instance always gets the same
// namespace, and that a long name still ends with that instance's id.
func TestManagedNamespaceNameStable(t *testing.T) {
	// the same instance returns one name
	id := uint(1219111753715908609)
	first := managedNamespaceName("wanaware-core", id)
	second := managedNamespaceName("wanaware-core", id)
	if first != second {
		t.Fatalf("retry got %q then %q", first, second)
	}
	if managedNamespaceName("wanaware-core", id+1) == first {
		t.Fatalf("a different instance reused %q", first)
	}
	if len(first) == 0 || len(first) > 63 {
		t.Fatalf("namespace length %d", len(first))
	}

	long := managedNamespaceName(strings.Repeat("n", 80), id)
	if len(long) > 63 {
		t.Fatalf("long name length %d", len(long))
	}
	if !strings.HasSuffix(long, "-"+strconv.FormatUint(uint64(id), 10)) {
		t.Fatalf("long name %q dropped the instance id", long)
	}
}
