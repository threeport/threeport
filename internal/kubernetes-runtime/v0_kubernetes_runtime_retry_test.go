package kubernetesruntime

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// apiCall is one call a kubernetes runtime reconciler makes to the
// Threeport API, by method and path.
type apiCall struct {
	method string
	path   string
}

// apiFault is how runtimeAPI fails the call a test picks.
type apiFault int

const (
	// faultNone answers every call.
	faultNone apiFault = iota
	// faultDropConnection closes the connection without answering, which
	// the client reports as a network error.
	faultDropConnection
	// faultServerError answers with the 500 the API server sends for an
	// internal error, which is not a network error.
	faultServerError
)

// apiServerErrorMessage is the internal error the API answers a call
// failed with faultServerError with.
const apiServerErrorMessage = "database is unavailable"

// runtimeAPI stands in for the Threeport API the kubernetes runtime
// reconcilers call: the kubernetes runtime definition 3 on infraProvider with
// accountName as its infra provider account, its provider definition 4, the
// GCP provider 1, and the provider instance 9 of kubernetes runtime instance 5.  The call that matches failing is failed with
// fault instead of answered.  It returns the calls it received.
func runtimeAPI(
	t *testing.T,
	infraProvider string,
	accountName *string,
	failing apiCall,
	fault apiFault,
) (*controller.Reconciler, func() []apiCall) {
	t.Helper()

	var mu sync.Mutex
	var calls []apiCall
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		call := apiCall{method: r.Method, path: r.URL.Path}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()

		if fault != faultNone && call == failing {
			if fault == faultServerError {
				body, err := json.Marshal(apiserver_lib.CreateResponseWithError500(
					nil, errors.New(apiServerErrorMessage), "",
				))
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(body)
				return
			}
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			require.NoError(t, conn.Close())
			return
		}

		var object apiserver_lib.Object
		switch {
		case strings.HasPrefix(call.path, v0.PathKubernetesRuntimeDefinitions):
			object = v0.KubernetesRuntimeDefinition{
				Common:                   v0.Common{ID: util.Ptr(uint(3))},
				InfraProvider:            util.Ptr(infraProvider),
				InfraProviderAccountName: accountName,
			}
		case strings.HasPrefix(call.path, v0.PathAwsEksKubernetesRuntimeDefinitions),
			strings.HasPrefix(call.path, v0.PathGcpGkeKubernetesRuntimeDefinitions),
			strings.HasPrefix(call.path, v0.PathOciOkeKubernetesRuntimeDefinitions):
			object = map[string]any{"ID": 4, "Name": "remote", "DeletionProtection": false}
		case strings.HasPrefix(call.path, v0.PathGcpProviders):
			object = v0.GcpProvider{Common: v0.Common{ID: util.Ptr(uint(1))}, Name: util.Ptr("account")}
		case strings.HasPrefix(call.path, v0.PathAwsEksKubernetesRuntimeInstances),
			strings.HasPrefix(call.path, v0.PathGcpGkeKubernetesRuntimeInstances),
			strings.HasPrefix(call.path, v0.PathOciOkeKubernetesRuntimeInstances):
			object = map[string]any{
				"ID":                                  9,
				"AwsEksKubernetesRuntimeDefinitionID": 4,
				"GcpGkeKubernetesRuntimeDefinitionID": 4,
				"OciOkeKubernetesRuntimeDefinitionID": 4,
			}
		default:
			t.Errorf("unexpected API call %s %s", call.method, call.path)
			http.NotFound(w, r)
			return
		}
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		machinetest.WriteResponse(t, w, status, []apiserver_lib.Object{object})
	})

	return &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}, func() []apiCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]apiCall(nil), calls...)
	}
}

// assertServerErrorReached checks that a call failed with faultServerError
// failed on the API's status, which carries its message, rather than on a body
// the client could not read.
func assertServerErrorReached(t *testing.T, fault apiFault, err error) {
	t.Helper()

	if fault != faultServerError {
		return
	}
	assert.Contains(t, err.Error(), apiServerErrorMessage)
	assert.NotContains(t, err.Error(), "failed to unmarshal")
}

// reconcileStep is one API call of a reconcile, and the message the
// reconciler wraps its failure in.
type reconcileStep struct {
	call    apiCall
	message string
}

// assertEachStepRetriesOnlyANetworkError fails each step of a reconcile in
// turn.  A dropped connection requeues the object after 30 seconds, while an
// error answer from the API stops with no requeue and carries the API's
// message; either way the error says which step failed.  With nothing failed, the reconcile succeeds without a requeue.
func assertEachStepRetriesOnlyANetworkError(
	t *testing.T,
	infraProvider string,
	accountName *string,
	steps []reconcileStep,
	reconcile func(r *controller.Reconciler) (int64, error),
) {
	t.Helper()

	t.Run("nothing fails", func(t *testing.T) {
		r, calls := runtimeAPI(t, infraProvider, accountName, apiCall{}, faultNone)
		delay, err := reconcile(r)
		require.NoError(t, err)
		assert.Zero(t, delay)
		for _, step := range steps {
			assert.Contains(t, calls(), step.call, "the reconcile makes each step's call")
		}
	})

	for _, step := range steps {
		for _, c := range []struct {
			name      string
			fault     apiFault
			wantDelay int64
		}{
			{"dropped connection", faultDropConnection, 30},
			{"server error", faultServerError, 0},
		} {
			t.Run(step.message+": "+c.name, func(t *testing.T) {
				r, calls := runtimeAPI(t, infraProvider, accountName, step.call, c.fault)
				delay, err := reconcile(r)
				require.Error(t, err)
				assert.Contains(t, err.Error(), step.message)
				assertServerErrorReached(t, c.fault, err)
				assert.Equal(t, c.wantDelay, delay)
				assert.Equal(t, step.call, calls()[len(calls())-1], "the reconcile stops at the failed call")
			})
		}
	}
}

// TestKubernetesRuntimeDefinitionCreatedRetriesOnlyANetworkError covers the
// call that creates the provider definition for a new EKS or GKE kubernetes
// runtime definition.
func TestKubernetesRuntimeDefinitionCreatedRetriesOnlyANetworkError(t *testing.T) {
	for infraProvider, step := range map[string]reconcileStep{
		v0.KubernetesRuntimeInfraProviderEKS: {
			apiCall{http.MethodPost, v0.PathAwsEksKubernetesRuntimeDefinitions},
			"failed to create new AWS EKS kubernetes runtime",
		},
		v0.KubernetesRuntimeInfraProviderGKE: {
			apiCall{http.MethodPost, v0.PathGcpGkeKubernetesRuntimeDefinitions},
			"failed to create new GCP GKE kubernetes runtime definition",
		},
	} {
		t.Run(infraProvider, func(t *testing.T) {
			assertEachStepRetriesOnlyANetworkError(t, infraProvider, util.Ptr("account"), []reconcileStep{step}, func(r *controller.Reconciler) (int64, error) {
				log := logr.Discard()
				return v0KubernetesRuntimeDefinitionCreated(r, &v0.KubernetesRuntimeDefinition{
					Common:           v0.Common{ID: util.Ptr(uint(3))},
					Definition:       v0.Definition{Name: util.Ptr("remote")},
					Reconciliation:   v0.Reconciliation{Reconciled: util.Ptr(false)},
					InfraProvider:    util.Ptr(infraProvider),
					HighAvailability: util.Ptr(false),
					NodeSize:         util.Ptr("Medium"),
					NodeProfile:      util.Ptr("Balanced"),
					NodeMaximum:      util.Ptr(10),
				}, &log)
			})
		})
	}
}

// TestKubernetesRuntimeInstanceCreatedRetriesOnlyANetworkError covers each
// call that creates the provider instance for a new EKS or GKE kubernetes
// runtime instance.
func TestKubernetesRuntimeInstanceCreatedRetriesOnlyANetworkError(t *testing.T) {
	definition := reconcileStep{
		apiCall{http.MethodGet, v0.PathKubernetesRuntimeDefinitions + "/3"},
		"failed to get kubernetes runtime definition by ID",
	}
	gkeDefinition := reconcileStep{
		apiCall{http.MethodGet, v0.PathGcpGkeKubernetesRuntimeDefinitions},
		"failed to get GCP GKE runtime definition by kubernetes runtime definition ID",
	}
	gkeInstance := reconcileStep{
		apiCall{http.MethodPost, v0.PathGcpGkeKubernetesRuntimeInstances},
		"failed to create GKE kubernetes runtime instance",
	}
	for _, c := range []struct {
		name          string
		infraProvider string
		accountName   *string
		steps         []reconcileStep
	}{
		{"eks", v0.KubernetesRuntimeInfraProviderEKS, util.Ptr("account"), []reconcileStep{
			definition,
			{apiCall{http.MethodGet, v0.PathAwsEksKubernetesRuntimeDefinitions}, "failed to get aws eks runtime definition by kubernetes runtime definition ID"},
			{apiCall{http.MethodPost, v0.PathAwsEksKubernetesRuntimeInstances}, "failed to create EKS kubernetes runtime instance"},
		}},
		{"gke on a named account", v0.KubernetesRuntimeInfraProviderGKE, util.Ptr("account"), []reconcileStep{
			definition,
			gkeDefinition,
			{apiCall{http.MethodGet, v0.PathGcpProviders}, "failed to get GCP provider by provider name"},
			gkeInstance,
		}},
		{"gke on the default account", v0.KubernetesRuntimeInfraProviderGKE, nil, []reconcileStep{
			definition,
			gkeDefinition,
			{apiCall{http.MethodGet, v0.PathGcpProviders}, "failed to get GCP provider by default"},
			gkeInstance,
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			assertEachStepRetriesOnlyANetworkError(t, c.infraProvider, c.accountName, c.steps, func(r *controller.Reconciler) (int64, error) {
				log := logr.Discard()
				return v0KubernetesRuntimeInstanceCreated(r, &v0.KubernetesRuntimeInstance{
					Common:                        v0.Common{ID: util.Ptr(uint(kubernetesRuntimeInstanceID))},
					Instance:                      v0.Instance{Name: util.Ptr("remote")},
					Reconciliation:                v0.Reconciliation{Reconciled: util.Ptr(false)},
					Location:                      util.Ptr("NorthAmerica:NewYork"),
					KubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
				}, &log)
			})
		})
	}
}

// TestKubernetesRuntimeInstanceDeletedRetriesOnlyANetworkError covers each
// call that deletes the provider instance of an EKS, GKE or OKE kubernetes
// runtime instance.
func TestKubernetesRuntimeInstanceDeletedRetriesOnlyANetworkError(t *testing.T) {
	definition := reconcileStep{
		apiCall{http.MethodGet, v0.PathKubernetesRuntimeDefinitions + "/3"},
		"failed to retrieve kubernetes runtime definition by ID",
	}
	for infraProvider, steps := range map[string][]reconcileStep{
		v0.KubernetesRuntimeInfraProviderEKS: {
			definition,
			{apiCall{http.MethodGet, v0.PathAwsEksKubernetesRuntimeInstances}, "failed to get aws eks runtime instance by kubernetes runtime instance ID"},
			{apiCall{http.MethodGet, v0.PathAwsEksKubernetesRuntimeDefinitions + "/4"}, "failed to get aws eks runtime definition for its deletion protection"},
			{apiCall{http.MethodDelete, v0.PathAwsEksKubernetesRuntimeInstances + "/9"}, "failed to delete aws eks runtime instance by ID"},
		},
		v0.KubernetesRuntimeInfraProviderGKE: {
			definition,
			{apiCall{http.MethodGet, v0.PathGcpGkeKubernetesRuntimeInstances}, "failed to get GCP GKE runtime instance by kubernetes runtime instance ID"},
			{apiCall{http.MethodGet, v0.PathGcpGkeKubernetesRuntimeDefinitions + "/4"}, "failed to get GCP GKE runtime definition for its deletion protection"},
			{apiCall{http.MethodDelete, v0.PathGcpGkeKubernetesRuntimeInstances + "/9"}, "failed to delete GCP GKE runtime instance by ID"},
		},
		v0.KubernetesRuntimeInfraProviderOKE: {
			definition,
			{apiCall{http.MethodGet, v0.PathOciOkeKubernetesRuntimeInstances}, "failed to get OCI OKE runtime instance by kubernetes runtime instance ID"},
			{apiCall{http.MethodGet, v0.PathOciOkeKubernetesRuntimeDefinitions + "/4"}, "failed to get OCI OKE runtime definition for its deletion protection"},
			{apiCall{http.MethodDelete, v0.PathOciOkeKubernetesRuntimeInstances + "/9"}, "failed to delete OCI OKE runtime instance by ID"},
		},
	} {
		t.Run(infraProvider, func(t *testing.T) {
			assertEachStepRetriesOnlyANetworkError(t, infraProvider, util.Ptr("account"), steps, func(r *controller.Reconciler) (int64, error) {
				log := logr.Discard()
				return v0KubernetesRuntimeInstanceDeleted(r, &v0.KubernetesRuntimeInstance{
					Common:                        v0.Common{ID: util.Ptr(uint(kubernetesRuntimeInstanceID))},
					KubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
					Reconciliation:                v0.Reconciliation{DeletionScheduled: util.Ptr(time.Now().UTC())},
				}, &log)
			})
		})
	}
}

// TestKubernetesRuntimeInstanceUpdatedWaitsForItsEndpoint covers how an
// update of a kubernetes runtime instance handles its API endpoint, up to
// the client it reaches the cluster with: one that is not stored yet is
// waited for, and one with no host fails the reconcile.  A network error
// reading the definition, or reaching the cluster, is retried, while an
// error answer is not.
func TestKubernetesRuntimeInstanceUpdatedWaitsForItsEndpoint(t *testing.T) {
	definitionCall := apiCall{http.MethodGet, v0.PathKubernetesRuntimeDefinitions + "/3"}
	update := func(r *controller.Reconciler, endpoint *string) (int64, error) {
		log := logr.Discard()
		return v0KubernetesRuntimeInstanceUpdated(r, &v0.KubernetesRuntimeInstance{
			Common:                        v0.Common{ID: util.Ptr(uint(kubernetesRuntimeInstanceID))},
			APIEndpoint:                   endpoint,
			ConnectionToken:               util.Ptr("token"),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
		}, &log)
	}
	eks := v0.KubernetesRuntimeInfraProviderEKS
	account := util.Ptr("account")

	t.Run("no endpoint yet", func(t *testing.T) {
		r, calls := runtimeAPI(t, eks, account, apiCall{}, faultNone)
		delay, err := update(r, nil)
		require.NoError(t, err)
		assert.Equal(t, controller.Requeue30s, delay, "polled until the endpoint is stored")
		assert.Empty(t, calls(), "nothing is read before the endpoint is stored")
	})
	t.Run("an endpoint with no host", func(t *testing.T) {
		r, calls := runtimeAPI(t, eks, account, apiCall{}, faultNone)
		delay, err := update(r, util.Ptr("https://"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `kubernetes runtime API endpoint "https://" has no host`)
		assert.Zero(t, delay, "the error counts toward the reconcile's failure limit, not a poll")
		assert.Empty(t, calls())
	})
	for _, c := range []struct {
		name      string
		fault     apiFault
		wantDelay int64
	}{
		{"dropped connection reading the definition", faultDropConnection, 30},
		{"server error reading the definition", faultServerError, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, calls := runtimeAPI(t, eks, account, definitionCall, c.fault)
			delay, err := update(r, util.Ptr("https://remote.example.com:443"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to retrieve kubernetes runtime definition by ID")
			assertServerErrorReached(t, c.fault, err)
			assert.Equal(t, c.wantDelay, delay)
			assert.Equal(t, []apiCall{definitionCall}, calls())
		})
	}

	t.Run("a cluster that cannot be reached", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closed := "http://" + listener.Addr().String()
		require.NoError(t, listener.Close())

		r, _ := runtimeAPI(t, eks, account, apiCall{}, faultNone)
		delay, err := update(r, &closed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get a Kubernetes client and mapper")
		assert.Equal(t, int64(30), delay)
	})
	t.Run("a cluster that answers with an error", func(t *testing.T) {
		cluster := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}))
		t.Cleanup(cluster.Close)

		r, _ := runtimeAPI(t, eks, account, apiCall{}, faultNone)
		delay, err := update(r, &cluster.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get a Kubernetes client and mapper")
		assert.Zero(t, delay)
	})
}
