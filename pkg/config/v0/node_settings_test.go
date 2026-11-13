package v0

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestConfigsSetTheNodeSizeProfileAndMaximum covers NodeSize, NodeProfile
// and NodeMaximum in the KubernetesRuntimeDefinition and KubernetesRuntime
// config files: a create sends them, the definition it returns shows them,
// and a replace that leaves them out keeps the stored ones.
func TestConfigsSetTheNodeSizeProfileAndMaximum(t *testing.T) {
	api := machinetest.NewAPIStub(t)
	var sent []api_v0.KubernetesRuntimeDefinition
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		var definition api_v0.KubernetesRuntimeDefinition
		require.NoError(t, json.NewDecoder(r.Body).Decode(&definition))
		sent = append(sent, definition)
		definition.ID = util.Ptr(uint(3))
		definition.CreatedAt = util.Ptr(time.Now())
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{definition})
	})

	definitionConfig := KubernetesRuntimeDefinitionConfig{KubernetesRuntimeDefinition: KubernetesRuntimeDefinitionValues{
		Name:          util.Ptr("production"),
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
		NodeSize:      util.Ptr("Large"),
		NodeProfile:   util.Ptr("Memory"),
		NodeMaximum:   util.Ptr(30),
	}}
	created, err := definitionConfig.Create(api.Client, api.Addr)
	require.NoError(t, err)
	assert.Equal(t, util.Ptr("Large"), sent[0].NodeSize)
	assert.Equal(t, util.Ptr("Memory"), sent[0].NodeProfile)
	assert.Equal(t, util.Ptr(30), sent[0].NodeMaximum)
	assert.Equal(t, util.Ptr("Large"), created.KubernetesRuntimeDefinition.NodeSize)
	assert.Equal(t, util.Ptr("Memory"), created.KubernetesRuntimeDefinition.NodeProfile)
	assert.Equal(t, util.Ptr(30), created.KubernetesRuntimeDefinition.NodeMaximum)

	runtimeConfig := KubernetesRuntimeConfig{KubernetesRuntime: KubernetesRuntimeValues{
		Name:          util.Ptr("production"),
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
		NodeSize:      util.Ptr("Small"),
		NodeProfile:   util.Ptr("Balanced"),
		NodeMaximum:   util.Ptr(12),
	}}
	operations, definitions, _ := runtimeConfig.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())
	assert.Equal(t, util.Ptr("Small"), sent[1].NodeSize)
	assert.Equal(t, util.Ptr("Balanced"), sent[1].NodeProfile)
	assert.Equal(t, util.Ptr(12), sent[1].NodeMaximum)
	assert.Equal(t, util.Ptr(12), (*definitions)[0].KubernetesRuntimeDefinition.NodeMaximum)

	stored := api_v0.KubernetesRuntimeDefinition{
		Common:        api_v0.Common{ID: util.Ptr(uint(1))},
		Definition:    api_v0.Definition{Name: util.Ptr("production")},
		InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
		NodeSize:      util.Ptr("Large"),
		NodeProfile:   util.Ptr("Memory"),
		NodeMaximum:   util.Ptr(30),
	}
	replace := func(values KubernetesRuntimeDefinitionValues) api_v0.KubernetesRuntimeDefinition {
		fake := &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathKubernetesRuntimeDefinitions: stored}}
		_, err := (&KubernetesRuntimeDefinitionConfig{KubernetesRuntimeDefinition: values}).Replace(&http.Client{}, fake.serve(), "production")
		require.NoError(t, err)
		var replaced api_v0.KubernetesRuntimeDefinition
		fake.decodeSent(&replaced)
		return replaced
	}
	kept := replace(KubernetesRuntimeDefinitionValues{
		Name: util.Ptr("production"), InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
	})
	assert.Equal(t, util.Ptr("Large"), kept.NodeSize, "a replace without them keeps them")
	assert.Equal(t, util.Ptr("Memory"), kept.NodeProfile)
	assert.Equal(t, util.Ptr(30), kept.NodeMaximum)
	changed := replace(KubernetesRuntimeDefinitionValues{
		Name: util.Ptr("production"), InfraProvider: util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
		NodeSize: util.Ptr("XLarge"), NodeProfile: util.Ptr("Balanced"), NodeMaximum: util.Ptr(40),
	})
	assert.Equal(t, util.Ptr("XLarge"), changed.NodeSize)
	assert.Equal(t, util.Ptr("Balanced"), changed.NodeProfile)
	assert.Equal(t, util.Ptr(40), changed.NodeMaximum)
}
