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

// providerDefinitionDeletes are the EKS and OKE definition configs, whose
// provider definition marries a kubernetes runtime definition as GKE's does.
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

// marriedDefinitionsAPI stands in for the Threeport API holding a provider
// definition 2 named dev-01, at providerPath, married to kubernetes runtime
// definition 9.  As the API does for a caller outside the control plane, it
// refuses to delete the runtime definition while the provider definition
// still references it.  runtimeDeleteStatus, when not zero, answers the
// runtime definition delete instead.  It returns the calls it received.
func marriedDefinitionsAPI(
	t *testing.T,
	providerPath string,
	providerPresent bool,
	runtimeDeleteStatus int,
) (string, func() []string) {
	t.Helper()

	name := "dev-01"
	providerID, runtimeID := uint(2), uint(9)
	provider := map[string]any{"ID": providerID, "Name": name, "KubernetesRuntimeDefinitionID": runtimeID}
	runtime := api_v0.KubernetesRuntimeDefinition{
		Common:     api_v0.Common{ID: &runtimeID},
		Definition: api_v0.Definition{Name: &name},
	}

	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, r.Method+" "+r.URL.Path)

		respond := func(status int, message string, data ...apiserver_lib.Object) {
			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{Code: status, Message: http.StatusText(status), Error: message},
				Data:   append([]apiserver_lib.Object{}, data...),
			})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(body)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == providerPath:
			if !providerPresent {
				respond(http.StatusOK, "")
				return
			}
			respond(http.StatusOK, "", provider)
		case r.Method == http.MethodDelete && r.URL.Path == providerPath+"/2":
			providerPresent = false
			respond(http.StatusOK, "", provider)
		case r.Method == http.MethodDelete && r.URL.Path == api_v0.PathKubernetesRuntimeDefinitions+"/9":
			switch {
			case providerPresent:
				respond(http.StatusConflict, "kubernetes runtime definition "+api_v0.ErrMsgDeleteBlocked+": it is married to a provider definition")
			case runtimeDeleteStatus != 0:
				respond(runtimeDeleteStatus, "runtime definition delete failed")
			default:
				respond(http.StatusOK, "", runtime)
			}
		default:
			t.Errorf("unexpected API call %s %s", r.Method, r.URL.Path)
			respond(http.StatusNotFound, "unexpected call")
		}
	}))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://"), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

// TestAProviderDefinitionIsDeletedBeforeTheRuntimeDefinitionItMarries
// covers #767: deleting an EKS or OKE kubernetes runtime definition by name
// deletes the provider definition first, then the kubernetes runtime
// definition at the ID the provider definition stored.  The other order is
// refused by the API with a 409, because the provider definition still
// references the runtime definition.
func TestAProviderDefinitionIsDeletedBeforeTheRuntimeDefinitionItMarries(t *testing.T) {
	for provider, config := range providerDefinitionDeletes {
		t.Run(provider, func(t *testing.T) {
			endpoint, calls := marriedDefinitionsAPI(t, config.path, true, 0)

			deletedName, err := config.deleteByName(http.DefaultClient, endpoint, "dev-01")
			require.NoError(t, err)
			assert.Equal(t, "dev-01", *deletedName)
			assert.Equal(t, []string{
				"GET " + config.path,
				"DELETE " + config.path + "/2",
				"DELETE " + api_v0.PathKubernetesRuntimeDefinitions + "/9",
			}, calls())
		})
	}
}

// TestARuntimeDefinitionAlreadyGoneDoesNotFailTheDelete covers a kubernetes
// runtime definition that is already gone once its provider definition is
// deleted: the delete succeeds.  Any other failure deleting it is reported.
func TestARuntimeDefinitionAlreadyGoneDoesNotFailTheDelete(t *testing.T) {
	for provider, config := range providerDefinitionDeletes {
		t.Run(provider+" already gone", func(t *testing.T) {
			endpoint, _ := marriedDefinitionsAPI(t, config.path, true, http.StatusNotFound)

			_, err := config.deleteByName(http.DefaultClient, endpoint, "dev-01")
			require.NoError(t, err)
		})
		t.Run(provider+" failing", func(t *testing.T) {
			endpoint, _ := marriedDefinitionsAPI(t, config.path, true, http.StatusInternalServerError)

			_, err := config.deleteByName(http.DefaultClient, endpoint, "dev-01")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to delete associated kubernetes runtime definition")
		})
	}
}

// TestAMissingProviderDefinitionDeletesNothing covers deleting an EKS or OKE
// definition whose provider definition does not exist: the config values are
// returned and nothing is deleted, as for GKE.  Its runtime definition, if
// a failed earlier delete left one, is not found by name: #768 deletes both
// in one request.
func TestAMissingProviderDefinitionDeletesNothing(t *testing.T) {
	for provider, config := range providerDefinitionDeletes {
		t.Run(provider, func(t *testing.T) {
			endpoint, calls := marriedDefinitionsAPI(t, config.path, false, 0)

			deletedName, err := config.deleteByName(http.DefaultClient, endpoint, "dev-01")
			require.NoError(t, err)
			assert.Equal(t, "dev-01", *deletedName)
			assert.Equal(t, []string{"GET " + config.path}, calls())
		})
	}
}
