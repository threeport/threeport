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

// eksPerZoneValues returns the values of an EKS definition config with
// perZone set.
func eksPerZoneValues(perZone *bool) AwsEksKubernetesRuntimeDefinitionValues {
	return AwsEksKubernetesRuntimeDefinitionValues{
		Name: util.Ptr("eks-remote"), ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("m8i.large"),
		DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(10),
		NodeGroupPerZone: perZone,
	}
}

// TestAnEksConfigSetsANodeGroupPerZone covers NodeGroupPerZone in the EKS
// config files: the definition config and the runtime config send it, and a
// replace that leaves it out keeps the stored value.
func TestAnEksConfigSetsANodeGroupPerZone(t *testing.T) {
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{api_v0.KubernetesRuntimeDefinition{
			Common: api_v0.Common{ID: util.Ptr(uint(2)), CreatedAt: util.Ptr(time.Now())},
		}})
	})
	var sent []api_v0.AwsEksKubernetesRuntimeDefinition
	api.Mux.HandleFunc(api_v0.PathAwsEksKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		var definition api_v0.AwsEksKubernetesRuntimeDefinition
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&definition))
		sent = append(sent, definition)
		definition.ID = util.Ptr(uint(3))
		definition.CreatedAt = util.Ptr(time.Now())
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{definition})
	})

	config := AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: eksPerZoneValues(util.Ptr(true))}
	created, err := config.Create(api.Client, api.Addr)
	require.NoError(t, err)
	assert.Equal(t, util.Ptr(true), created.AwsEksKubernetesRuntimeDefinition.NodeGroupPerZone)

	values := eksPerZoneValues(util.Ptr(true))
	runtime := AwsEksKubernetesRuntimeConfig{AwsEksKubernetesRuntime: AwsEksKubernetesRuntimeValues{
		Name: values.Name, ZoneCount: values.ZoneCount, DefaultNodeGroupInstanceType: values.DefaultNodeGroupInstanceType,
		DefaultNodeGroupInitialSize: values.DefaultNodeGroupInitialSize, DefaultNodeGroupMinimumSize: values.DefaultNodeGroupMinimumSize,
		DefaultNodeGroupMaximumSize: values.DefaultNodeGroupMaximumSize, NodeGroupPerZone: util.Ptr(true),
	}}
	operations, _, _ := runtime.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())

	require.Len(t, sent, 2)
	for _, definition := range sent {
		assert.Equal(t, util.Ptr(true), definition.NodeGroupPerZone)
	}

	stored := api_v0.AwsEksKubernetesRuntimeDefinition{
		Common: api_v0.Common{ID: util.Ptr(uint(1))}, Definition: api_v0.Definition{Name: util.Ptr("eks-remote")},
		NodeGroupPerZone: util.Ptr(true),
	}
	fake := &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathAwsEksKubernetesRuntimeDefinitions: stored}}
	replace := AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: eksPerZoneValues(nil)}
	_, err = replace.Replace(&http.Client{}, fake.serve(), "eks-remote")
	require.NoError(t, err)
	var replaced api_v0.AwsEksKubernetesRuntimeDefinition
	fake.decodeSent(&replaced)
	assert.Equal(t, util.Ptr(true), replaced.NodeGroupPerZone, "a config that leaves it out keeps it")
}
