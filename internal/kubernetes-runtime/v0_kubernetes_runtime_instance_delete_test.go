package kubernetesruntime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	// kubernetesRuntimeInstanceID is the ID of the kubernetes runtime instance
	// under delete in these tests.
	kubernetesRuntimeInstanceID = 5

	// providerRecordID is the ID of the provider-specific record that
	// providerRecordServer serves when the record is present.
	providerRecordID = 9
)

// providerRecordGoneServer stands in for the Threeport API when a kubernetes
// runtime instance's provider-specific record has already been deleted. The
// definition is served by ID with the given infra provider; the provider
// instance filter answers 200 with an empty Data, which is how the API reports
// no match. It records every request so a test can check nothing was deleted.
func providerRecordGoneServer(t *testing.T, infraProvider string) (*httptest.Server, func() []string) {
	t.Helper()
	return providerRecordServer(t, infraProvider, false)
}

// providerRecordServer is providerRecordGoneServer with the choice of whether
// the provider-specific record still exists. When it does, the provider
// instance filter answers with one object whose ID is providerRecordID, but only
// when it is filtered by kubernetesruntimeinstanceid=kubernetesRuntimeInstanceID;
// any other filter finds nothing, so a handler that looks the record up by the
// wrong ID cannot reach it. The provider instance DELETE answers with the object
// so a delete of it can succeed.
func providerRecordServer(t *testing.T, infraProvider string, recordPresent bool) (*httptest.Server, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var requests []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.Contains(r.URL.Path, "kubernetes-runtime-definitions"):
			fmt.Fprintf(w, `{"Data":[{"ID":3,"InfraProvider":%q}]}`, infraProvider)
		case recordPresent && r.Method == http.MethodDelete,
			recordPresent && r.URL.Query().Get("kubernetesruntimeinstanceid") == strconv.Itoa(kubernetesRuntimeInstanceID):
			fmt.Fprintf(w, `{"Data":[{"ID":%d}]}`, providerRecordID)
		default:
			fmt.Fprint(w, `{"Data":[]}`)
		}
	}))
	t.Cleanup(server.Close)

	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), requests...)
	}
}

// TestKubernetesRuntimeInstanceDeleted_ProviderRecordAlreadyGone is the
// regression test for a delete that requeued forever: the provider-specific
// instance had been deleted, its lookup came back empty, and the handler
// returned an error instead of letting the delete be confirmed.
func TestKubernetesRuntimeInstanceDeleted_ProviderRecordAlreadyGone(t *testing.T) {
	for _, infraProvider := range []string{
		v0.KubernetesRuntimeInfraProviderGKE,
		v0.KubernetesRuntimeInfraProviderEKS,
		v0.KubernetesRuntimeInfraProviderOKE,
	} {
		t.Run(infraProvider, func(t *testing.T) {
			server, requests := providerRecordGoneServer(t, infraProvider)
			r := &controller.Reconciler{
				APIServer: strings.TrimPrefix(server.URL, "http://"),
				APIClient: server.Client(),
			}
			instance := &v0.KubernetesRuntimeInstance{
				Common:                        v0.Common{ID: util.Ptr(uint(kubernetesRuntimeInstanceID))},
				KubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
				Reconciliation: v0.Reconciliation{
					DeletionScheduled: util.Ptr(time.Now().UTC()),
				},
			}
			log := logr.Discard()

			requeueDelay, err := v0KubernetesRuntimeInstanceDeleted(r, instance, &log)

			require.NoError(t, err, "a provider record that is already gone must not fail the delete")
			assert.Zero(t, requeueDelay, "no requeue, so the reconciler confirms the deletion")
			for _, request := range requests() {
				assert.False(t, strings.HasPrefix(request, http.MethodDelete), "nothing is left to delete: %s", request)
			}
		})
	}
}

// TestKubernetesRuntimeInstanceDeleted_ProviderRecordPresent checks that a
// delete reaches the provider-specific record when it still exists. Without
// that call the cloud resource is orphaned while the kubernetes runtime
// instance is confirmed deleted - which is what happened for OKE, whose
// provider was missing from the handler's switch.
func TestKubernetesRuntimeInstanceDeleted_ProviderRecordPresent(t *testing.T) {
	for infraProvider, path := range map[string]string{
		v0.KubernetesRuntimeInfraProviderEKS: v0.PathAwsEksKubernetesRuntimeInstances,
		v0.KubernetesRuntimeInfraProviderGKE: v0.PathGcpGkeKubernetesRuntimeInstances,
		v0.KubernetesRuntimeInfraProviderOKE: v0.PathOciOkeKubernetesRuntimeInstances,
	} {
		t.Run(infraProvider, func(t *testing.T) {
			server, requests := providerRecordServer(t, infraProvider, true)
			r := &controller.Reconciler{
				APIServer: strings.TrimPrefix(server.URL, "http://"),
				APIClient: server.Client(),
			}
			instance := &v0.KubernetesRuntimeInstance{
				Common:                        v0.Common{ID: util.Ptr(uint(kubernetesRuntimeInstanceID))},
				KubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
				Reconciliation: v0.Reconciliation{
					DeletionScheduled: util.Ptr(time.Now().UTC()),
				},
			}
			log := logr.Discard()

			requeueDelay, err := v0KubernetesRuntimeInstanceDeleted(r, instance, &log)

			require.NoError(t, err)
			assert.Zero(t, requeueDelay)
			assert.Contains(
				t,
				requests(),
				fmt.Sprintf("%s %s/%d", http.MethodDelete, path, providerRecordID),
				"the provider-specific record must be deleted",
			)
		})
	}
}
