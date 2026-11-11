package gcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAGkeClusterIsLaidOutInItsAllocatedRange covers the infra the GKE
// adapter builds: a cluster yet to be provisioned is allocated a range and
// laid out in it, and one already provisioned keeps what its runtime records.
func TestAGkeClusterIsLaidOutInItsAllocatedRange(t *testing.T) {
	serviceAccountCredentials := `{"type":"service_account","project_id":"some-project","client_email":"threeport@some-project.iam.gserviceaccount.com"}`
	encryptionKey, storedCredentials := encryptedCredentials(t, serviceAccountCredentials)

	var mu sync.Mutex
	runtime := api_v0.KubernetesRuntimeInstance{Common: api_v0.Common{ID: util.Ptr(uint(5))}}
	respond := func(w http.ResponseWriter, objects ...apiserver_lib.Object) {
		body, err := json.Marshal(apiserver_lib.Response{Status: apiserver_lib.Status{Code: http.StatusOK}, Data: objects})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, api_v0.PathGcpProviders):
			respond(w, api_v0.GcpProvider{
				Common: api_v0.Common{ID: util.Ptr(uint(1))}, ProjectID: util.Ptr("some-project"), ServiceAccountCredentials: &storedCredentials,
			})
		case strings.HasPrefix(r.URL.Path, api_v0.PathGcpGkeKubernetesRuntimeDefinitions):
			respond(w, api_v0.GcpGkeKubernetesRuntimeDefinition{
				Common: api_v0.Common{ID: util.Ptr(uint(3))}, DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
				DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			})
		case strings.HasPrefix(r.URL.Path, api_v0.PathControlPlaneInstances):
			respond(w, api_v0.ControlPlaneInstance{Common: api_v0.Common{ID: util.Ptr(uint(1))}, IsSelf: util.Ptr(true)})
		case r.URL.Path == api_v0.PathKubernetesRuntimeInstances:
			respond(w, runtime)
		case strings.HasPrefix(r.URL.Path, api_v0.PathKubernetesRuntimeInstances+"/"):
			if r.Method == http.MethodPatch {
				var update api_v0.KubernetesRuntimeInstance
				require.NoError(t, json.NewDecoder(r.Body).Decode(&update))
				runtime.NetworkCidr = update.NetworkCidr
			}
			respond(w, runtime)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	lifecycle := &gkeLifecycle{
		r: &controller.Reconciler{
			APIClient: &http.Client{}, APIServer: strings.TrimPrefix(srv.URL, "http://"), EncryptionKey: encryptionKey,
		},
		instanceID: 7,
	}
	instance := &api_v0.GcpGkeKubernetesRuntimeInstance{
		Common:                              api_v0.Common{ID: util.Ptr(uint(7))},
		Instance:                            api_v0.Instance{Name: util.Ptr("gke")},
		Region:                              util.Ptr("us-central1"),
		GcpProviderID:                       util.Ptr(uint(1)),
		GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
		KubernetesRuntimeInstanceID:         util.Ptr(uint(5)),
	}

	// a provisioned cluster created before ranges were allocated gets none
	inventory := datatypes.JSON(`{"deployment":{"resources":[{"urn":"urn:subnet"}]}}`)
	instance.ResourceInventory = &inventory
	instance.CreationConfirmed = util.Ptr(time.Now())
	infra, err := lifecycle.buildInfraFrom(instance)
	require.NoError(t, err)
	assert.Empty(t, infra.NetworkCidr)
	assert.Nil(t, runtime.NetworkCidr, "nothing is allocated to it")

	// a new one is allocated a range and laid out in it
	instance.ResourceInventory = nil
	instance.CreationConfirmed = nil
	infra, err = lifecycle.buildInfraFrom(instance)
	require.NoError(t, err)
	assert.Equal(t, "10.16.0.0/16", infra.NetworkCidr)
	assert.Equal(t, util.Ptr("10.16.0.0/16"), runtime.NetworkCidr)
}
