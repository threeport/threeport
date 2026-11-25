package gcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// the GKE adapter has its clusters upgraded
var _ provider.UpgradeLifecycleProvider = (*gkeLifecycle)(nil)

// gkeUpgradeAPI serves a GKE instance and its definition, and records the
// instance updates it is sent.
func gkeUpgradeAPI(
	t *testing.T,
	instance api_v0.GcpGkeKubernetesRuntimeInstance,
	definition api_v0.GcpGkeKubernetesRuntimeDefinition,
) (*gkeLifecycle, func() []map[string]any) {
	t.Helper()

	var mu sync.Mutex
	var updates []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var object any = instance
		if strings.Contains(r.URL.Path, "definitions") {
			object = definition
		}
		if r.Method == http.MethodPatch {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var update map[string]any
			require.NoError(t, json.Unmarshal(body, &update))
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		}
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{object},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	lifecycle := &gkeLifecycle{
		r: &controller.Reconciler{
			APIClient: &http.Client{},
			APIServer: strings.TrimPrefix(server.URL, "http://"),
		},
		instanceID: *instance.ID,
	}

	return lifecycle, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), updates...)
	}
}

// TestTheGkeInstanceRecordsItsUpgrade covers the upgrade state the GKE
// adapter reads from and writes to the API.
func TestTheGkeInstanceRecordsItsUpgrade(t *testing.T) {
	acknowledged := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	instance := api_v0.GcpGkeKubernetesRuntimeInstance{
		Common:                              api_v0.Common{ID: util.Ptr(uint(7))},
		GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
		KubernetesVersion:                   util.Ptr("1.35"),
		UpgradeAcknowledged:                 &acknowledged,
		Upgrading:                           util.Ptr(true),
		UpgradeFailed:                       util.Ptr(false),
		UpgradeStatus:                       util.Ptr("upgrading from Kubernetes 1.35 to 1.36"),
	}
	definition := api_v0.GcpGkeKubernetesRuntimeDefinition{
		Common:            api_v0.Common{ID: util.Ptr(uint(3))},
		KubernetesVersion: util.Ptr("1.36"),
	}
	lifecycle, updates := gkeUpgradeAPI(t, instance, definition)

	snapshot, err := lifecycle.GetUpgrade()
	require.NoError(t, err)
	assert.Equal(t, "1.35", snapshot.RecordedVersion)
	assert.True(t, snapshot.Acknowledged.Equal(acknowledged))
	assert.True(t, snapshot.Upgrading)
	assert.False(t, snapshot.Failed)
	assert.False(t, snapshot.Refused, "unset is not refused")
	assert.Equal(t, "upgrading from Kubernetes 1.35 to 1.36", snapshot.Status)

	target, err := lifecycle.GetUpgradeTarget()
	require.NoError(t, err)
	assert.Equal(t, "1.36", target)

	require.NoError(t, lifecycle.AckUpgrade("upgrading"))
	require.NoError(t, lifecycle.FinishUpgrade(provider.UpgradeOutcome{Version: "1.36", Status: "upgraded"}))
	require.NoError(t, lifecycle.FinishUpgrade(provider.UpgradeOutcome{Refused: true, Status: "refused"}))
	require.NoError(t, lifecycle.RecordUpgradeStatus("1.37", "newer"))
	require.NoError(t, lifecycle.OnUpgraded(nil))

	sent := updates()
	require.Len(t, sent, 4, "GKE upgrades the nodes itself, so nothing follows an upgrade")
	assert.Equal(t, true, sent[0]["Upgrading"])
	assert.Equal(t, false, sent[0]["UpgradeFailed"])
	assert.Equal(t, false, sent[0]["UpgradeRefused"])
	assert.Equal(t, "upgrading", sent[0]["UpgradeStatus"])
	assert.NotNil(t, sent[0]["UpgradeAcknowledged"])

	assert.Equal(t, false, sent[1]["Upgrading"])
	assert.Equal(t, "1.36", sent[1]["KubernetesVersion"])
	assert.Equal(t, "upgraded", sent[1]["UpgradeStatus"])

	assert.Equal(t, true, sent[2]["UpgradeRefused"])
	assert.NotContains(t, sent[2], "KubernetesVersion", "an unknown version isn't recorded")

	assert.Equal(t, "1.37", sent[3]["KubernetesVersion"])
	assert.Equal(t, "newer", sent[3]["UpgradeStatus"])
	assert.NotContains(t, sent[3], "Upgrading")

	// a definition created before versions were recorded asks for the default
	definition.KubernetesVersion = nil
	lifecycle, _ = gkeUpgradeAPI(t, instance, definition)
	target, err = lifecycle.GetUpgradeTarget()
	require.NoError(t, err)
	assert.Equal(t, api_v0.DefaultKubernetesVersion, target)
}
