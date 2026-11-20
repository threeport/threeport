package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// providerDefinitionDelete is how a test deletes one provider's kubernetes
// runtime definition by name through its config.
type providerDefinitionDelete struct {
	// path is the API path of the provider's definitions
	path string
	// deleteByName deletes the provider definition with the given name and
	// returns the name the deleted config reports
	deleteByName func(apiClient *http.Client, apiEndpoint, name string) (*string, error)
}

// providerDefinitionDeletes are the EKS, GKE and OKE definition configs,
// whose provider definition marries a kubernetes runtime definition.
var providerDefinitionDeletes = map[string]providerDefinitionDelete{
	"eks": {
		path: api_v0.PathAwsEksKubernetesRuntimeDefinitions,
		deleteByName: func(apiClient *http.Client, apiEndpoint, name string) (*string, error) {
			config := &AwsEksKubernetesRuntimeDefinitionConfig{
				AwsEksKubernetesRuntimeDefinition: AwsEksKubernetesRuntimeDefinitionValues{Name: &name},
			}
			deleted, err := config.Delete(apiClient, apiEndpoint)
			if err != nil {
				return nil, err
			}
			return deleted.AwsEksKubernetesRuntimeDefinition.Name, nil
		},
	},
	"gke": {
		path: api_v0.PathGcpGkeKubernetesRuntimeDefinitions,
		deleteByName: func(apiClient *http.Client, apiEndpoint, name string) (*string, error) {
			config := &GcpGkeKubernetesRuntimeDefinitionConfig{
				GcpGkeKubernetesRuntimeDefinition: GcpGkeKubernetesRuntimeDefinitionValues{Name: &name},
			}
			deleted, err := config.Delete(apiClient, apiEndpoint)
			if err != nil {
				return nil, err
			}
			return deleted.GcpGkeKubernetesRuntimeDefinition.Name, nil
		},
	},
	"oke": {
		path: api_v0.PathOciOkeKubernetesRuntimeDefinitions,
		deleteByName: func(apiClient *http.Client, apiEndpoint, name string) (*string, error) {
			config := &OciOkeKubernetesRuntimeDefinitionConfig{
				OciOkeKubernetesRuntimeDefinition: OciOkeKubernetesRuntimeDefinitionValues{Name: &name},
			}
			deleted, err := config.Delete(apiClient, apiEndpoint)
			if err != nil {
				return nil, err
			}
			return deleted.OciOkeKubernetesRuntimeDefinition.Name, nil
		},
	},
}

// providerDefinitionAPI stands in for the Threeport API holding, when
// present, a provider definition 2 named dev-01 at providerPath, married to
// kubernetes runtime definition 9.  It answers the provider definition's
// lookup and delete, and refuses any other call.  It returns the calls it
// received.
func providerDefinitionAPI(t *testing.T, providerPath string, present bool) (string, func() []string) {
	t.Helper()

	provider := map[string]any{"ID": 2, "Name": "dev-01", "KubernetesRuntimeDefinitionID": 9}
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()

		var data []apiserver_lib.Object
		switch {
		case r.Method == http.MethodGet && r.URL.Path == providerPath:
			if present {
				data = append(data, provider)
			}
		case r.Method == http.MethodDelete && r.URL.Path == providerPath+"/2":
			data = append(data, provider)
		default:
			t.Errorf("unexpected API call %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected call", http.StatusNotFound)
			return
		}
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   data,
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://"), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

// TestAProviderDefinitionIsDeletedWithOneRequest covers #768: deleting an
// EKS, GKE or OKE kubernetes runtime definition by name makes one DELETE, of
// the provider definition.  The API deletes the kubernetes runtime definition
// it marries in the same transaction, so the client has no second delete
// that could fail after the first succeeded.
func TestAProviderDefinitionIsDeletedWithOneRequest(t *testing.T) {
	for provider, config := range providerDefinitionDeletes {
		t.Run(provider, func(t *testing.T) {
			endpoint, calls := providerDefinitionAPI(t, config.path, true)

			deletedName, err := config.deleteByName(http.DefaultClient, endpoint, "dev-01")
			require.NoError(t, err)
			assert.Equal(t, "dev-01", *deletedName)
			assert.Equal(t, []string{
				"GET " + config.path,
				"DELETE " + config.path + "/2",
			}, calls())
		})
	}
}

// TestAMissingProviderDefinitionDeletesNothing covers deleting an EKS, GKE or
// OKE definition whose provider definition does not exist: the config values
// are returned and nothing is deleted.
func TestAMissingProviderDefinitionDeletesNothing(t *testing.T) {
	for provider, config := range providerDefinitionDeletes {
		t.Run(provider, func(t *testing.T) {
			endpoint, calls := providerDefinitionAPI(t, config.path, false)

			deletedName, err := config.deleteByName(http.DefaultClient, endpoint, "dev-01")
			require.NoError(t, err)
			assert.Equal(t, "dev-01", *deletedName)
			assert.Equal(t, []string{"GET " + config.path}, calls())
		})
	}
}
