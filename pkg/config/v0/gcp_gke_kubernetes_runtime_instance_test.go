package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestGcpGkeKubernetesRuntimeInstanceConfig_Replace_PreservesForeignKeys is a
// regression test for a bug where GcpProviderID was dropped from the PUT
// payload sent by Replace(), causing the API to reject every replace call
// with a 400. It asserts all three foreign keys the existing object carries
// (GcpProviderID, KubernetesRuntimeInstanceID, GcpGkeKubernetesRuntimeDefinitionID)
// are present in the outgoing PUT body, so a future edit that drops one of
// them again is caught here rather than only in production.
func TestGcpGkeKubernetesRuntimeInstanceConfig_Replace_PreservesForeignKeys(t *testing.T) {
	existingID := uint(1)
	gcpProviderID := uint(101)
	kubernetesRuntimeInstanceID := uint(102)
	definitionID := uint(103)
	name := "test-instance"
	region := "us-east1"

	createdAt := time.Now()
	existing := api_v0.GcpGkeKubernetesRuntimeInstance{
		Common:                              api_v0.Common{ID: &existingID, CreatedAt: &createdAt},
		Instance:                            api_v0.Instance{Name: &name},
		Region:                              &region,
		GcpProviderID:                       &gcpProviderID,
		KubernetesRuntimeInstanceID:         &kubernetesRuntimeInstanceID,
		GcpGkeKubernetesRuntimeDefinitionID: &definitionID,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			// Replace() looks up the existing object by name first.
			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
				Data:   []apiserver_lib.Object{existing},
			})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)

		case r.Method == http.MethodPut:
			// this is the payload under test: assert every foreign key
			// carried on the existing object survived into the PUT body.
			var payload api_v0.GcpGkeKubernetesRuntimeInstance
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))

			assert.Equal(t, &gcpProviderID, payload.GcpProviderID, "GcpProviderID must be preserved in the replace payload")
			assert.Equal(t, &kubernetesRuntimeInstanceID, payload.KubernetesRuntimeInstanceID, "KubernetesRuntimeInstanceID must be preserved in the replace payload")
			assert.Equal(t, &definitionID, payload.GcpGkeKubernetesRuntimeDefinitionID, "GcpGkeKubernetesRuntimeDefinitionID must be preserved in the replace payload")

			// the client strips CreatedAt from the outgoing PUT payload since
			// it isn't settable; a real API response would still include it.
			payload.CreatedAt = &createdAt

			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
				Data:   []apiserver_lib.Object{payload},
			})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	config := &GcpGkeKubernetesRuntimeInstanceConfig{
		GcpGkeKubernetesRuntimeInstance: GcpGkeKubernetesRuntimeInstanceValues{
			Name:            &name,
			Region:          &region,
			GcpProviderName: util.Ptr("some-provider"),
			GcpGkeKubernetesRuntimeDefinition: &GcpGkeKubernetesRuntimeDefinitionValues{
				Name: util.Ptr("some-definition"),
			},
		},
	}

	_, err := config.Replace(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"), name)
	require.NoError(t, err, fmt.Sprintf("Replace should succeed against %s", srv.URL))
}
