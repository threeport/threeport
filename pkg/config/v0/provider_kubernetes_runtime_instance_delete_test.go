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

// TestAProviderInstanceDeleteLeavesTheRuntimeInstanceToTheAPI covers #768:
// deleting an EKS or OKE kubernetes runtime instance by name deletes the
// provider instance and waits for it to go.  The API deletes the kubernetes
// runtime instance it marries when the provider instance's delete is
// confirmed, so the client makes no call to the kubernetes runtime instance.
func TestAProviderInstanceDeleteLeavesTheRuntimeInstanceToTheAPI(t *testing.T) {
	original := instanceDeleteWait
	t.Cleanup(func() { instanceDeleteWait = original })
	instanceDeleteWait = struct{ attempts, intervalSeconds int }{attempts: 3, intervalSeconds: 0}

	for provider, c := range map[string]struct {
		path   string
		delete func(apiClient *http.Client, apiEndpoint, name string) error
	}{
		"eks": {api_v0.PathAwsEksKubernetesRuntimeInstances, func(apiClient *http.Client, apiEndpoint, name string) error {
			config := AwsEksKubernetesRuntimeInstanceConfig{
				AwsEksKubernetesRuntimeInstance: AwsEksKubernetesRuntimeInstanceValues{Name: &name},
			}
			_, err := config.Delete(apiClient, apiEndpoint)
			return err
		}},
		"oke": {api_v0.PathOciOkeKubernetesRuntimeInstances, func(apiClient *http.Client, apiEndpoint, name string) error {
			config := OciOkeKubernetesRuntimeInstanceConfig{
				OciOkeKubernetesRuntimeInstance: OciOkeKubernetesRuntimeInstanceValues{Name: &name},
			}
			_, err := config.Delete(apiClient, apiEndpoint)
			return err
		}},
	} {
		t.Run(provider, func(t *testing.T) {
			instance := map[string]any{"ID": 7, "Name": "dev-01", "KubernetesRuntimeInstanceID": 9}
			var mu sync.Mutex
			var calls []string
			deleted := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				calls = append(calls, r.Method+" "+r.URL.Path)

				data := []apiserver_lib.Object{instance}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == c.path:
					if deleted {
						data = []apiserver_lib.Object{}
					}
				case r.Method == http.MethodDelete && r.URL.Path == c.path+"/7":
					deleted = true
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

			require.NoError(t, c.delete(http.DefaultClient, strings.TrimPrefix(server.URL, "http://"), "dev-01"))
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, []string{
				"GET " + c.path,
				"DELETE " + c.path + "/7",
				"GET " + c.path,
			}, calls)
		})
	}
}
