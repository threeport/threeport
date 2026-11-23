package v0

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/threeport/threeport/pkg/api/v0"
)

// TestRefreshManagedChildGenesisWritesTheServingName checks the child takes the serving genesis name.
func TestRefreshManagedChildGenesisWritesTheServingName(t *testing.T) {
	// serve the copied name, then the name the patch writes
	childName := "genesis-05"
	patches := 0
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			patches++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			if !strings.Contains(string(body), "genesis-06") {
				t.Errorf("patch body %s", body)
			}
			childName = "genesis-06"
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Data":[{"ID":8,"Name":%q,"Genesis":true,"IsSelf":false}]}`, childName)
	}))
	t.Cleanup(child.Close)
	childAddr := strings.TrimPrefix(child.URL, "http://")

	// list self, the managed child, a child with no address, and the genesis reference
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.RawQuery, "genesis=true") {
			fmt.Fprint(w, `{"Data":[{"ID":1,"Name":"genesis-06","Genesis":true,"IsSelf":true}]}`)
			return
		}
		fmt.Fprintf(w, `{"Meta":{"Pagination":{"HasMore":false}},"Data":[
			{"ID":1,"Name":"genesis-06","Genesis":true,"IsSelf":true},
			{"ID":2,"Name":"primary-12","ParentControlPlaneInstanceID":1,"ApiServerEndpoint":%q},
			{"ID":3,"Name":"primary-11","ParentControlPlaneInstanceID":1},
			{"ID":4,"Name":"genesis-06","Genesis":true,"IsSelf":false,"ApiServerEndpoint":"127.0.0.1:1"}
		]}`, childAddr)
	}))
	t.Cleanup(parent.Close)

	// write the serving name onto the managed child only
	selfID := uint(1)
	self := &api.ControlPlaneInstance{ID: &selfID}
	err := refreshManagedChildGenesis(
		parent.Client(),
		strings.TrimPrefix(parent.URL, "http://"),
		self,
	)
	if err != nil {
		t.Fatal(err)
	}
	if patches != 1 {
		t.Fatalf("patches %d", patches)
	}
}

// TestRefreshManagedChildGenesisLeavesAnUnreachableChild checks an offline child does not fail the restore.
func TestRefreshManagedChildGenesisLeavesAnUnreachableChild(t *testing.T) {
	// list one managed child whose API address accepts nothing
	queued := false
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Error(readErr)
			}
			compact := strings.ReplaceAll(string(body), " ", "")
			if strings.Contains(compact, `"Reconciled":false`) &&
				!strings.Contains(compact, "old-tag") &&
				!strings.Contains(compact, "child-01") {
				queued = true
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"Data":[{"ID":2,"Reconciled":false}]}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.RawQuery, "genesis=true") {
			fmt.Fprint(w, `{"Data":[{"ID":1,"Name":"parent-02","Genesis":true,"IsSelf":true}]}`)
			return
		}
		fmt.Fprint(w, `{"Meta":{"Pagination":{"HasMore":false}},"Data":[
			{"ID":1,"Name":"parent-02","Genesis":true,"IsSelf":true},
			{"ID":2,"Name":"child-01","ImageTag":"old-tag","Reconciled":true,"ParentControlPlaneInstanceID":1,"ApiServerEndpoint":"127.0.0.1:1"}
		]}`)
	}))
	t.Cleanup(parent.Close)

	// finish the restore and mark the child without its listed image
	selfID := uint(1)
	self := &api.ControlPlaneInstance{ID: &selfID}
	err := refreshManagedChildGenesis(
		parent.Client(),
		strings.TrimPrefix(parent.URL, "http://"),
		self,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("child was not marked unreconciled")
	}
}
