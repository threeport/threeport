package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// reclaimRequest is a controller as a registering module sends it.
func reclaimRequest() *v0.ModuleController {
	return &v0.ModuleController{
		Common:         v0.Common{ID: util.Ptr(uint(9))},
		Name:           util.Ptr("widget-controller"),
		DeploymentName: util.Ptr("ns/widget-controller"),
		ModuleApiID:    util.Ptr(uint(2)),
	}
}

// TestReclaimModuleControllerSendsOneRequest covers the single POST that
// replaces the separate owner check and update.
func TestReclaimModuleControllerSendsOneRequest(t *testing.T) {
	var requests []string
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Data":[{"ID":4,"Name":"widget-controller","DeploymentName":"ns/widget-controller","ModuleApiID":2}]}`)
	}))
	defer server.Close()

	reclaimed, err := ReclaimModuleController(server.Client(), apiAddr(server), reclaimRequest())

	require.NoError(t, err)
	assert.Equal(t, []string{"POST " + v0.PathModuleControllerReclaim}, requests)
	assert.NotContains(t, sent, "ID", "the caller's row ID is not part of the request")
	assert.Equal(t, uint(4), *reclaimed.ID)
	assert.Equal(t, uint(2), *reclaimed.ModuleApiID)
}

// TestReclaimModuleControllerReportsActiveOwnerAsConflict covers the 409 the
// server returns when another module api still owns the controller.
func TestReclaimModuleControllerReportsActiveOwnerAsConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"Status":{"Code":409,"Error":"module controller \"widget-controller\" is already owned by a different, active module api 1"}}`)
	}))
	defer server.Close()

	_, err := ReclaimModuleController(server.Client(), apiAddr(server), reclaimRequest())

	require.Error(t, err)
	assert.ErrorIs(t, err, client_lib.ErrConflict)
}
