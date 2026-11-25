package v0

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestRefreshRestoredGenesisLeavesADatabaseWithoutAGenesisRow covers a restore with nothing to write.
func TestRefreshRestoredGenesisLeavesADatabaseWithoutAGenesisRow(t *testing.T) {
	// a caller with no current genesis makes no request
	if err := RefreshRestoredGenesis(nil, "", nil); err != nil {
		t.Fatal(err)
	}

	// an empty database is left for the caller to create
	server := emptyDataServer(t)
	defer server.Close()
	_, err := GetGenesisControlPlaneInstance(server.Client(), apiAddr(server))
	if !errors.Is(err, ErrNoGenesisControlPlane) {
		t.Fatalf("missing genesis row: %v", err)
	}
	current := &api.ControlPlaneInstance{Name: util.Ptr("genesis-current")}
	if err := RefreshRestoredGenesis(server.Client(), apiAddr(server), current); err != nil {
		t.Fatal(err)
	}
}

// TestRefreshRestoredGenesisWritesTheServingName covers a copied row taking the current name.
func TestRefreshRestoredGenesisWritesTheServingName(t *testing.T) {
	// serve the copied name, then the name the patch writes
	const serving = "genesis-current"
	name := "genesis-05"
	patches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			patches++
			payload, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Error(readErr)
			}
			if !strings.Contains(string(payload), serving) {
				t.Errorf("patch body %s", payload)
			}
			name = serving
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Meta":{"ObjectCount":1},"Data":[{"ID":4,"Name":%q,"Genesis":true,"IsSelf":false}],"Status":{"code":200,"message":"OK"}}`, name)
	}))
	defer server.Close()

	// write the serving name onto the copied row
	current := &api.ControlPlaneInstance{Name: util.Ptr(serving)}
	if err := RefreshRestoredGenesis(server.Client(), apiAddr(server), current); err != nil {
		t.Fatal(err)
	}
	if patches != 1 {
		t.Fatalf("patches %d", patches)
	}

	// a second call leaves the matching row alone
	if err := RefreshRestoredGenesis(server.Client(), apiAddr(server), current); err != nil {
		t.Fatal(err)
	}
	if patches != 1 {
		t.Fatalf("second call patches %d", patches)
	}
}

// TestRefreshRestoredGenesisKeepsTheNameWhenTheImageIsRefused checks the name is stored when the image update is refused.
func TestRefreshRestoredGenesisKeepsTheNameWhenTheImageIsRefused(t *testing.T) {
	// serve the old image, accept the name, and refuse the new image
	const serving = "genesis-06"
	const oldTag = "v0.7.0-rc.5"
	const newTag = "v0.7.0-rc.6"
	patches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			patches++
			payload, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Error(readErr)
			}
			body := string(payload)
			if patches == 1 {
				if !strings.Contains(body, serving) || strings.Contains(body, newTag) {
					t.Errorf("name patch %s", body)
				}
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"Meta":{"ObjectCount":1},"Data":[{"ID":4,"Name":%q,"ImageTag":%q,"Genesis":true,"IsSelf":false}],"Status":{"code":200,"message":"OK"}}`, serving, oldTag)
				return
			}
			if !strings.Contains(body, newTag) {
				t.Errorf("image patch %s", body)
			}
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"Status":{"code":400,"error":"cannot update ImageTag on a control plane this API does not serve"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Meta":{"ObjectCount":1},"Data":[{"ID":4,"Name":"genesis-05","ImageTag":%q,"Genesis":true,"IsSelf":false}],"Status":{"code":200,"message":"OK"}}`, oldTag)
	}))
	defer server.Close()

	// store the name and continue when the image is refused
	current := &api.ControlPlaneInstance{
		Name:     util.Ptr(serving),
		ImageTag: util.Ptr(newTag),
	}
	if err := RefreshRestoredGenesis(server.Client(), apiAddr(server), current); err != nil {
		t.Fatal(err)
	}
	if patches != 2 {
		t.Fatalf("patches %d", patches)
	}
}

// TestRefreshRestoredGenesisReturnsAnImageWriteFailure checks an image update error is returned.
func TestRefreshRestoredGenesisReturnsAnImageWriteFailure(t *testing.T) {
	// accept the name and fail the image write
	const serving = "genesis-06"
	const oldTag = "v0.7.0-rc.5"
	const newTag = "v0.7.0-rc.6"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			payload, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Error(readErr)
			}
			if strings.Contains(string(payload), newTag) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"Status":{"code":500,"error":"database unavailable"}}`)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"Meta":{"ObjectCount":1},"Data":[{"ID":4,"Name":%q,"ImageTag":%q,"Genesis":true,"IsSelf":false}],"Status":{"code":200,"message":"OK"}}`, serving, oldTag)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Meta":{"ObjectCount":1},"Data":[{"ID":4,"Name":"genesis-05","ImageTag":%q,"Genesis":true,"IsSelf":false}],"Status":{"code":200,"message":"OK"}}`, oldTag)
	}))
	defer server.Close()

	// return the image update error
	current := &api.ControlPlaneInstance{
		Name:     util.Ptr(serving),
		ImageTag: util.Ptr(newTag),
	}
	if err := RefreshRestoredGenesis(server.Client(), apiAddr(server), current); err == nil {
		t.Fatal("image update error was ignored")
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
