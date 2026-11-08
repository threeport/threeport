package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestGcpGkeKubernetesRuntimeDefinitionConfig_DeleteDeletesProviderDefinitionFirst checks delete order.
// The GKE definition is deleted before the kubernetes runtime definition it references.
func TestGcpGkeKubernetesRuntimeDefinitionConfig_DeleteDeletesProviderDefinitionFirst(t *testing.T) {
	name := "dev-01"
	definitionID := uint(2)
	kubernetesRuntimeDefinitionID := uint(9)
	existing := api_v0.GcpGkeKubernetesRuntimeDefinition{
		Common:                        api_v0.Common{ID: &definitionID},
		Definition:                    api_v0.Definition{Name: &name},
		KubernetesRuntimeDefinitionID: &kubernetesRuntimeDefinitionID,
	}
	runtimeDefinition := api_v0.KubernetesRuntimeDefinition{
		Common:     api_v0.Common{ID: &kubernetesRuntimeDefinitionID},
		Definition: api_v0.Definition{Name: &name},
	}

	// record each call and answer with the object that call deletes
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)

		var payload any
		switch {
		case r.Method == http.MethodGet:
			payload = existing
		case r.URL.Path == api_v0.PathGcpGkeKubernetesRuntimeDefinitions+"/2":
			payload = existing
		case r.URL.Path == api_v0.PathKubernetesRuntimeDefinitions+"/9":
			payload = runtimeDefinition
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}

		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{payload},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
		require.NoError(t, err)
	}))
	defer server.Close()

	// delete the definition by name
	config := &GcpGkeKubernetesRuntimeDefinitionConfig{
		GcpGkeKubernetesRuntimeDefinition: GcpGkeKubernetesRuntimeDefinitionValues{
			Name: &name,
		},
	}
	_, err := config.Delete(http.DefaultClient, strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)

	// the provider definition delete precedes the kubernetes runtime definition delete
	require.Equal(t, []string{
		"GET " + api_v0.PathGcpGkeKubernetesRuntimeDefinitions,
		"DELETE " + api_v0.PathGcpGkeKubernetesRuntimeDefinitions + "/2",
		"DELETE " + api_v0.PathKubernetesRuntimeDefinitions + "/9",
	}, calls)
}
