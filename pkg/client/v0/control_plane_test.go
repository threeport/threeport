package v0

import (
	"testing"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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

// TestRefreshCopiedGenesisWritesTheCurrentIdentity covers a restored genesis row and a self row.
func TestRefreshCopiedGenesisWritesTheCurrentIdentity(t *testing.T) {
	// a copied genesis row takes the current name, address, image, and certificate
	definitionID := uint(12)
	stored := &api.ControlPlaneInstance{
		ID:                          util.Ptr(uint(4)),
		ControlPlaneDefinitionID:    &definitionID,
		Name:                        util.Ptr("genesis-05"),
		Namespace:                   util.Ptr("genesis-05"),
		ApiServerEndpoint:           util.Ptr("10.0.0.5:443"),
		ImageNamespace:              util.Ptr("repo.example/threeport"),
		ImageTag:                    util.Ptr("253721d6"),
		KubernetesRuntimeInstanceID: util.Ptr(uint(9)),
		ClientKey:                   util.Ptr("key-stored"),
	}
	current := &api.ControlPlaneInstance{
		Name:              util.Ptr("genesis-07"),
		Namespace:         util.Ptr("genesis-07"),
		ApiServerEndpoint: util.Ptr("10.0.0.7:443"),
		ImageNamespace:    util.Ptr("repo.example/threeport"),
		ImageTag:          util.Ptr("2026-11-21.a0f923a0"),
		CACert:            util.Ptr("ca-current"),
		ClientKey:         util.Ptr(""),
	}
	updated, changed := refreshCopiedGenesis(stored, current)
	if !changed {
		t.Fatal("restored genesis row was left unchanged")
	}
	if updated.Name == nil || *updated.Name != "genesis-07" {
		t.Fatal("restored genesis name was not refreshed")
	}
	if updated.Namespace == nil || *updated.Namespace != "genesis-07" {
		t.Fatal("restored genesis namespace was not refreshed")
	}
	if updated.ApiServerEndpoint == nil || *updated.ApiServerEndpoint != "10.0.0.7:443" {
		t.Fatal("restored genesis address was not refreshed")
	}
	if updated.ImageTag == nil || *updated.ImageTag != "2026-11-21.a0f923a0" {
		t.Fatal("restored genesis image was not refreshed")
	}
	if updated.CACert == nil || *updated.CACert != "ca-current" {
		t.Fatal("restored genesis certificate was not refreshed")
	}
	if updated.ControlPlaneDefinitionID == nil || *updated.ControlPlaneDefinitionID != definitionID {
		t.Fatal("restored genesis definition changed")
	}
	if updated.KubernetesRuntimeInstanceID == nil || *updated.KubernetesRuntimeInstanceID != 9 {
		t.Fatal("restored genesis runtime changed")
	}
	if updated.ClientKey == nil || *updated.ClientKey != "key-stored" {
		t.Fatal("empty credential wiped the stored one")
	}

	// the replacement's own row stays as it is
	self := true
	own := &api.ControlPlaneInstance{IsSelf: &self, Name: util.Ptr("primary-14")}
	kept, changed := refreshCopiedGenesis(own, current)
	if changed || kept.Name == nil || *kept.Name != "primary-14" {
		t.Fatal("self row was treated as the copied genesis row")
	}

	// a row that already matches is not written again
	_, changed = refreshCopiedGenesis(updated, current)
	if changed {
		t.Fatal("matching genesis row was marked changed")
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
