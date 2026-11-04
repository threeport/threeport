package v0

import (
	"encoding/json"
	"fmt"
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
	util "github.com/threeport/threeport/pkg/util/v0"
)

// controllerState is everything on a runtime instance that its controller
// writes and no caller of Replace is setting.
type controllerState struct {
	Reconciliation    api_v0.Reconciliation
	Status            *string
	ResourceInventory *datatypes.JSON
}

// seedControllerState returns the state of a cluster that was created and
// confirmed, with every reconciliation field set to something other than nil.
// A Replace that leaves any one of them off its payload writes nil over it, so
// a value in each is what lets the assertion see which one went missing. The
// deletion fields are not what a healthy cluster carries, but a PUT writes them
// all the same, and the point is that none of them is dropped.
func seedControllerState() controllerState {
	at := func(minute int) *time.Time {
		return util.Ptr(time.Date(2026, 10, 1, 12, minute, 0, 0, time.UTC))
	}
	inventory := datatypes.JSON(`{"version":3,"deployment":{"resources":[` +
		`{"urn":"urn:pulumi:dev::runtime::cloud:network/network:Network::vpc"},` +
		`{"urn":"urn:pulumi:dev::runtime::cloud:cluster/cluster:Cluster::cluster"}]}}`)

	return controllerState{
		Reconciliation: api_v0.Reconciliation{
			Reconciled:              util.Ptr(true),
			CreationAcknowledged:    at(1),
			CreationConfirmed:       at(2),
			CreationFailed:          util.Ptr(false),
			DeletionScheduled:       at(3),
			DeletionAcknowledged:    at(4),
			DeletionConfirmed:       at(5),
			InterruptReconciliation: util.Ptr(true),
		},
		Status:            util.Ptr("Running"),
		ResourceInventory: &inventory,
	}
}

// assertControllerStateCarried checks that what a Replace sent is what the
// object already held. Comparing the Reconciliation struct whole means a field
// added to it later is covered without this test being edited.
func assertControllerStateCarried(t *testing.T, want, got controllerState) {
	t.Helper()

	assert.Equal(
		t, want.Reconciliation, got.Reconciliation,
		"every Reconciliation field must be carried over: a lost CreationConfirmed "+
			"is what lets a later delete destroy an empty stack and confirm",
	)
	assert.Equal(t, want.Status, got.Status, "Status must be carried over")
	if assert.NotNil(t, got.ResourceInventory, "ResourceInventory must be carried over: it is the only record of the cloud resources") {
		assert.JSONEq(t, string(*want.ResourceInventory), string(*got.ResourceInventory))
	}
}

// serveReplace stands in for the Threeport API for one runtime instance
// Replace: it serves the existing object by name, serves whatever related
// objects Replace looks up on the way, and records the PUT body it is sent.
//
// The mock echoes the existing object back from the PUT. What a Replace sends is
// what is under test, so the response only has to be something Replace can build
// its config from.
//
// Errors are collected rather than raised from the handler goroutine, because
// require and t.Fatalf may only be called from the goroutine running the test.
func serveReplace[T any](
	t *testing.T,
	instancePath string,
	existing T,
	related map[string]any,
) (endpoint string, sent func() T) {
	t.Helper()

	var (
		mutex    sync.Mutex
		put      *T
		failures []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fail := func(format string, args ...any) {
			message := fmt.Sprintf(format, args...)
			mutex.Lock()
			failures = append(failures, message)
			mutex.Unlock()
			http.Error(w, message, http.StatusInternalServerError)
		}
		respond := func(object any) {
			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
				Data:   []apiserver_lib.Object{object},
			})
			if err != nil {
				fail("failed to marshal response: %v", err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == instancePath:
			respond(existing)

		case r.Method == http.MethodGet && related[r.URL.Path] != nil:
			respond(related[r.URL.Path])

		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, instancePath+"/"):
			var payload T
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				fail("failed to decode PUT request body: %v", err)
				return
			}
			mutex.Lock()
			put = &payload
			mutex.Unlock()
			respond(existing)

		default:
			fail("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://"), func() T {
		t.Helper()
		mutex.Lock()
		defer mutex.Unlock()
		require.Empty(t, failures, "the mock API server was sent a request it did not expect")
		require.NotNil(t, put, "Replace never sent a PUT")

		return *put
	}
}

func TestGcpGkeKubernetesRuntimeInstanceConfig_Replace_CarriesControllerState(t *testing.T) {
	state := seedControllerState()
	existingID := uint(1)
	providerID, runtimeInstanceID, definitionID := uint(101), uint(102), uint(103)
	createdAt := time.Now()
	name := "test-instance"

	existing := api_v0.GcpGkeKubernetesRuntimeInstance{
		Common:                              api_v0.Common{ID: &existingID, CreatedAt: &createdAt},
		Instance:                            api_v0.Instance{Name: &name, Status: state.Status},
		Reconciliation:                      state.Reconciliation,
		Region:                              util.Ptr("us-east1"),
		GcpProviderID:                       &providerID,
		KubernetesRuntimeInstanceID:         &runtimeInstanceID,
		GcpGkeKubernetesRuntimeDefinitionID: &definitionID,
		ResourceInventory:                   state.ResourceInventory,
	}
	endpoint, sent := serveReplace(t, api_v0.PathGcpGkeKubernetesRuntimeInstances, existing, nil)

	config := &GcpGkeKubernetesRuntimeInstanceConfig{
		GcpGkeKubernetesRuntimeInstance: GcpGkeKubernetesRuntimeInstanceValues{
			Name:            &name,
			Region:          util.Ptr("us-west2"),
			GcpProviderName: util.Ptr("some-provider"),
			GcpGkeKubernetesRuntimeDefinition: &GcpGkeKubernetesRuntimeDefinitionValues{
				Name: util.Ptr("some-definition"),
			},
		},
	}
	_, err := config.Replace(&http.Client{}, endpoint, name)
	require.NoError(t, err)

	payload := sent()
	assertControllerStateCarried(t, state, controllerState{
		Reconciliation:    payload.Reconciliation,
		Status:            payload.Status,
		ResourceInventory: payload.ResourceInventory,
	})
	assert.Equal(t, util.Ptr("us-west2"), payload.Region, "a field the caller sets must still change")
}

func TestAwsEksKubernetesRuntimeInstanceConfig_Replace_CarriesControllerState(t *testing.T) {
	state := seedControllerState()
	existingID := uint(1)
	providerID, runtimeInstanceID, definitionID := uint(101), uint(102), uint(103)
	createdAt := time.Now()
	name := "test-instance"

	existing := api_v0.AwsEksKubernetesRuntimeInstance{
		Common:                              api_v0.Common{ID: &existingID, CreatedAt: &createdAt},
		Instance:                            api_v0.Instance{Name: &name, Status: state.Status},
		Reconciliation:                      state.Reconciliation,
		Region:                              util.Ptr("us-east-1"),
		AwsProviderID:                       &providerID,
		KubernetesRuntimeInstanceID:         &runtimeInstanceID,
		AwsEksKubernetesRuntimeDefinitionID: &definitionID,
		ResourceInventory:                   state.ResourceInventory,
	}
	endpoint, sent := serveReplace(t, api_v0.PathAwsEksKubernetesRuntimeInstances, existing, nil)

	config := &AwsEksKubernetesRuntimeInstanceConfig{
		AwsEksKubernetesRuntimeInstance: AwsEksKubernetesRuntimeInstanceValues{
			Name:            &name,
			Region:          util.Ptr("us-west-2"),
			AwsProviderName: util.Ptr("some-provider"),
			AwsEksKubernetesRuntimeDefinition: &AwsEksKubernetesRuntimeDefinitionValues{
				Name: util.Ptr("some-definition"),
			},
		},
	}
	_, err := config.Replace(&http.Client{}, endpoint, name)
	require.NoError(t, err)

	payload := sent()
	assertControllerStateCarried(t, state, controllerState{
		Reconciliation:    payload.Reconciliation,
		Status:            payload.Status,
		ResourceInventory: payload.ResourceInventory,
	})
	assert.Equal(t, util.Ptr("us-west-2"), payload.Region, "a field the caller sets must still change")
}

// TestOciOkeKubernetesRuntimeInstanceConfig_Replace_CarriesControllerState also
// covers what is particular to OKE: ClusterOCID, which the controller fills in
// once the cluster exists, and the runtime instance foreign key, which Replace
// used to leave off. The API treats a nil foreign key on a PUT as clearing a
// relationship that is immutable once set, and rejects the request.
func TestOciOkeKubernetesRuntimeInstanceConfig_Replace_CarriesControllerState(t *testing.T) {
	state := seedControllerState()
	existingID := uint(1)
	providerID, runtimeInstanceID, definitionID := uint(201), uint(202), uint(203)
	createdAt := time.Now()
	name := "test-instance"
	clusterOCID := "ocid1.cluster.oc1.phx.aaaaaaaaexample"

	existing := api_v0.OciOkeKubernetesRuntimeInstance{
		Common:                              api_v0.Common{ID: &existingID, CreatedAt: &createdAt},
		Instance:                            api_v0.Instance{Name: &name, Status: state.Status},
		Reconciliation:                      state.Reconciliation,
		Region:                              util.Ptr("us-phoenix-1"),
		OciProviderID:                       &providerID,
		KubernetesRuntimeInstanceID:         &runtimeInstanceID,
		OciOkeKubernetesRuntimeDefinitionID: &definitionID,
		ResourceInventory:                   state.ResourceInventory,
		ClusterOCID:                         &clusterOCID,
	}
	endpoint, sent := serveReplace(
		t,
		api_v0.PathOciOkeKubernetesRuntimeInstances,
		existing,
		map[string]any{
			// Replace resolves the names the config carries to IDs
			api_v0.PathOciProviders: api_v0.OciProvider{
				Common: api_v0.Common{ID: &providerID},
			},
			api_v0.PathOciOkeKubernetesRuntimeDefinitions: api_v0.OciOkeKubernetesRuntimeDefinition{
				Common: api_v0.Common{ID: &definitionID},
			},
		},
	)

	config := &OciOkeKubernetesRuntimeInstanceConfig{
		OciOkeKubernetesRuntimeInstance: OciOkeKubernetesRuntimeInstanceValues{
			Name:            &name,
			Region:          util.Ptr("us-ashburn-1"),
			OciProviderName: util.Ptr("some-provider"),
			OciOkeKubernetesRuntimeDefinition: &OciOkeKubernetesRuntimeDefinitionValues{
				Name: util.Ptr("some-definition"),
			},
		},
	}
	_, err := config.Replace(&http.Client{}, endpoint, name)
	require.NoError(t, err)

	payload := sent()
	assertControllerStateCarried(t, state, controllerState{
		Reconciliation:    payload.Reconciliation,
		Status:            payload.Status,
		ResourceInventory: payload.ResourceInventory,
	})
	assert.Equal(t, &clusterOCID, payload.ClusterOCID, "ClusterOCID is the controller's and must be carried over")
	assert.Equal(t, &providerID, payload.OciProviderID, "OciProviderID must be preserved in the replace payload")
	assert.Equal(t, &runtimeInstanceID, payload.KubernetesRuntimeInstanceID,
		"KubernetesRuntimeInstanceID must be preserved in the replace payload: leaving it off is rejected as clearing an immutable relationship")
	assert.Equal(t, &definitionID, payload.OciOkeKubernetesRuntimeDefinitionID, "OciOkeKubernetesRuntimeDefinitionID must be preserved in the replace payload")
	assert.Equal(t, util.Ptr("us-ashburn-1"), payload.Region, "a field the caller sets must still change")
}
