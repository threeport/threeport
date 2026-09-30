package gcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// TestBuildGkeInfra_MapsDefinitionNodePoolFields is a regression test for the
// GcpGkeKubernetesRuntimeDefinition -> KubernetesRuntimeInfraGKE mapping in
// buildGkeInfra. It asserts MachineType, MinNodeCount, and MaxNodeCount are
// correctly read from the definition, since a prior version of this function
// silently dropped these fields, leaving the Pulumi node pool with hardcoded
// values regardless of what the definition requested.
func TestBuildGkeInfra_MapsDefinitionNodePoolFields(t *testing.T) {
	projectID := "some-project"
	gcpProviderID := uint(1)

	gcpProvider := api_v0.GcpProvider{
		Common:    api_v0.Common{ID: &gcpProviderID},
		ProjectID: &projectID,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{gcpProvider},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	r := &controller.Reconciler{
		APIClient: &http.Client{},
		APIServer: strings.TrimPrefix(srv.URL, "http://"),
	}

	instanceName := "test-instance"
	region := "us-east1"
	instance := &api_v0.GcpGkeKubernetesRuntimeInstance{
		Instance:      api_v0.Instance{Name: &instanceName},
		Region:        &region,
		GcpProviderID: &gcpProviderID,
	}

	machineType := "e2-standard-4"
	initialSize := 2
	minSize := 3
	maxSize := 7
	definition := &api_v0.GcpGkeKubernetesRuntimeDefinition{
		DefaultNodeGroupInstanceType: &machineType,
		DefaultNodeGroupInitialSize:  &initialSize,
		DefaultNodeGroupMinimumSize:  &minSize,
		DefaultNodeGroupMaximumSize:  &maxSize,
	}

	infra, err := buildGkeInfra(r, instance, definition, nil)
	require.NoError(t, err)

	assert.Equal(t, "e2-standard-4", infra.MachineType)
	assert.Equal(t, int32(2), infra.WorkerNodeInitialCount)
	assert.Equal(t, int32(3), infra.MinNodeCount)
	assert.Equal(t, int32(7), infra.MaxNodeCount)
}
