package v0

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// noDeleteWait removes the real poll interval for the duration of a test.
func noDeleteWait(t *testing.T) {
	t.Helper()
	zero := 0
	deleteWaitIntervalOverride = &zero
	t.Cleanup(func() { deleteWaitIntervalOverride = nil })
}

func TestWaitForInstanceDeleted(t *testing.T) {
	noDeleteWait(t)

	t.Run("not found means deleted", func(t *testing.T) {
		err := waitForInstanceDeleted("widget", "w1", 3, 1, func() error {
			return client_lib.ErrObjectNotFound
		})
		require.NoError(t, err)
	})

	t.Run("a wrapped not found means deleted", func(t *testing.T) {
		err := waitForInstanceDeleted("widget", "w1", 3, 1, func() error {
			return errors.Join(errors.New("context"), client_lib.ErrObjectNotFound)
		})
		require.NoError(t, err)
	})

	t.Run("an instance that never goes away is an error", func(t *testing.T) {
		calls := 0
		err := waitForInstanceDeleted("widget", "w1", 3, 1, func() error {
			calls++
			return nil
		})
		require.Error(t, err)
		assert.Equal(t, 3, calls)
		assert.Contains(t, err.Error(), "timed out waiting for widget instance w1 to be deleted")
		assert.Contains(t, err.Error(), "after 3s")
		assert.Contains(t, err.Error(), "still present")
	})

	t.Run("other errors are retried, not treated as deleted", func(t *testing.T) {
		calls := 0
		err := waitForInstanceDeleted("widget", "w1", 5, 1, func() error {
			calls++
			if calls < 3 {
				return errors.New("500 from API")
			}
			return client_lib.ErrObjectNotFound
		})
		require.NoError(t, err)
		assert.Equal(t, 3, calls)
	})

	t.Run("an API that keeps failing is an error that carries the API error", func(t *testing.T) {
		apiErr := errors.New("401 from API")
		err := waitForInstanceDeleted("widget", "w1", 3, 1, func() error {
			return apiErr
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, apiErr)
	})
}

// fakeDeleteAPI serves GET-by-name, GET-by-ID and DELETE for one API path and
// records every request it receives.
type fakeDeleteAPI struct {
	mu       sync.Mutex
	requests []string
	// getByName is called with the 1-based count of GET-by-name calls and
	// writes the response.
	getByName func(w http.ResponseWriter, call int)
	// object is returned for the DELETE.
	object   apiserver_lib.Object
	nameGets int
}

func (f *fakeDeleteAPI) handler(t *testing.T, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		var call int
		isNameGet := r.Method == http.MethodGet && r.URL.Path == path && r.URL.Query().Get("name") != ""
		if isNameGet {
			f.nameGets++
			call = f.nameGets
		}
		f.mu.Unlock()

		switch {
		case isNameGet:
			f.getByName(w, call)
		case r.Method == http.MethodDelete:
			respondWithObject(t, w, f.object)
		default:
			http.Error(w, "unexpected request "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}
}

func (f *fakeDeleteAPI) requestsMatching(sub string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matched []string
	for _, r := range f.requests {
		if strings.Contains(r, sub) {
			matched = append(matched, r)
		}
	}
	return matched
}

func respondWithNoObjects(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	respondWithObjects(t, w, []apiserver_lib.Object{})
}

func TestKubernetesWorkloadInstanceConfig_Delete_WaitsForTheObjectToBeGone(t *testing.T) {
	noDeleteWait(t)

	id := uint(7)
	name := "my-workload"
	instance := api_v0.KubernetesWorkloadInstance{
		Common:   api_v0.Common{ID: &id},
		Instance: api_v0.Instance{Name: &name},
	}
	cfg := &KubernetesWorkloadInstanceConfig{
		KubernetesWorkloadInstance: KubernetesWorkloadInstanceValues{Name: &name},
	}

	t.Run("errors when the object is still there after the wait", func(t *testing.T) {
		api := &fakeDeleteAPI{
			object: instance,
			getByName: func(w http.ResponseWriter, _ int) {
				respondWithObject(t, w, instance)
			},
		}
		srv := httptest.NewServer(api.handler(t, api_v0.PathKubernetesWorkloadInstances))
		defer srv.Close()

		deleted, err := cfg.Delete(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"))
		require.Error(t, err)
		assert.Nil(t, deleted)
		assert.Contains(t, err.Error(), "timed out waiting for kubernetes workload instance my-workload to be deleted")
		assert.Len(t, api.requestsMatching("DELETE"), 1)
	})

	t.Run("succeeds when polling hits a 500 and then the object is gone", func(t *testing.T) {
		api := &fakeDeleteAPI{
			object: instance,
			getByName: func(w http.ResponseWriter, call int) {
				switch call {
				case 1: // Delete's initial lookup
					respondWithObject(t, w, instance)
				case 2:
					http.Error(w, "boom", http.StatusInternalServerError)
				default:
					respondWithNoObjects(t, w)
				}
			},
		}
		srv := httptest.NewServer(api.handler(t, api_v0.PathKubernetesWorkloadInstances))
		defer srv.Close()

		deleted, err := cfg.Delete(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"))
		require.NoError(t, err)
		require.NotNil(t, deleted)
		assert.Equal(t, name, *deleted.KubernetesWorkloadInstance.Name)
		assert.Equal(t, 3, api.nameGets)
	})
}

// TestAwsEksKubernetesRuntimeInstanceConfig_Delete_TimeoutLeavesRuntimeInstanceAlone
// guards the EKS Delete against force-confirming and deleting the associated
// KubernetesRuntimeInstance while the EKS instance may still exist.
func TestAwsEksKubernetesRuntimeInstanceConfig_Delete_TimeoutLeavesRuntimeInstanceAlone(t *testing.T) {
	noDeleteWait(t)

	id := uint(3)
	runtimeID := uint(4)
	name := "my-eks"
	instance := api_v0.AwsEksKubernetesRuntimeInstance{
		Common:                      api_v0.Common{ID: &id},
		Instance:                    api_v0.Instance{Name: &name},
		KubernetesRuntimeInstanceID: &runtimeID,
	}
	cfg := &AwsEksKubernetesRuntimeInstanceConfig{
		AwsEksKubernetesRuntimeInstance: AwsEksKubernetesRuntimeInstanceValues{Name: &name},
	}

	api := &fakeDeleteAPI{
		object: instance,
		getByName: func(w http.ResponseWriter, _ int) {
			respondWithObject(t, w, instance)
		},
	}
	srv := httptest.NewServer(api.handler(t, api_v0.PathAwsEksKubernetesRuntimeInstances))
	defer srv.Close()

	deleted, err := cfg.Delete(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"))
	require.Error(t, err)
	assert.Nil(t, deleted)
	assert.Contains(t, err.Error(), "timed out waiting for AWS EKS kubernetes runtime instance my-eks to be deleted")
	assert.Empty(
		t,
		api.requestsMatching(api_v0.PathKubernetesRuntimeInstances),
		"the kubernetes runtime instance must not be read, confirmed or deleted after a timeout",
	)
}
