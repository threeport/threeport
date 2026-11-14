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

// TestReparentDetachedSkipsAncestors leaves a parent and a genesis copy alone.
func TestReparentDetachedSkipsAncestors(t *testing.T) {
	// a detached child points at self, and the reference rows do not
	self := uint(2)
	parent := uint(8)
	genesis := uint(1)
	primary := uint(5)
	missing := uint(9)
	genesisRow := true
	byID := map[uint]api.ControlPlaneInstance{
		self:    {ID: &self, ParentControlPlaneInstanceID: &parent},
		parent:  {ID: &parent},
		genesis: {ID: &genesis, Genesis: &genesisRow},
		primary: {ID: &primary, ParentControlPlaneInstanceID: &missing},
	}
	if reparentDetached(byID, self, byID[parent]) {
		t.Fatal("managing parent was treated as a child")
	}
	if reparentDetached(byID, self, byID[genesis]) {
		t.Fatal("genesis reference was treated as a child")
	}
	if !reparentDetached(byID, self, byID[primary]) {
		t.Fatal("detached child was left off self")
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
