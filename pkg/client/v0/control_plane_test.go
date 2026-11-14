package v0

import (
	"testing"

	api "github.com/threeport/threeport/pkg/api/v0"
)

// TestParentChainReaches covers a missing parent and a parent that is self.
func TestParentChainReaches(t *testing.T) {
	// a child of a missing parent does not reach self
	self := uint(1)
	primary := uint(5)
	missing := uint(9)
	if parentChainReaches(rows(self, nil, primary, &missing), primary, self) {
		t.Fatal("primary reached self through a missing parent")
	}
	if !parentChainReaches(rows(self, nil, primary, &self), primary, self) {
		t.Fatal("primary did not reach self")
	}
}

// rows builds the id map parentChainReaches walks.
func rows(self uint, selfParent *uint, primary uint, primaryParent *uint) map[uint]api.ControlPlaneInstance {
	// map each id to its parent
	return map[uint]api.ControlPlaneInstance{
		self:    {ID: &self, ParentControlPlaneInstanceID: selfParent},
		primary: {ID: &primary, ParentControlPlaneInstanceID: primaryParent},
	}
}
