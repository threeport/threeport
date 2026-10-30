package kubernetesruntime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

// providerRecordGoneServer stands in for the Threeport API when a kubernetes
// runtime instance's provider-specific record has already been deleted. The
// definition is served by ID with the given infra provider; the provider
// instance filter answers 200 with an empty Data, which is how the API reports
// no match. It records every request so a test can check nothing was deleted.
func providerRecordGoneServer(t *testing.T, infraProvider string) (*httptest.Server, func() []string) {
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
	} {
		t.Run(infraProvider, func(t *testing.T) {
			server, requests := providerRecordGoneServer(t, infraProvider)
			r := &controller.Reconciler{
				APIServer: strings.TrimPrefix(server.URL, "http://"),
				APIClient: server.Client(),
			}
			instance := &v0.KubernetesRuntimeInstance{
				Common:                        v0.Common{ID: util.Ptr(uint(5))},
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
