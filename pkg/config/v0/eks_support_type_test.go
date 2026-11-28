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

// eksDefinitionValues returns the values of an EKS definition config with
// the given support type.
func eksDefinitionValues(supportType *string) AwsEksKubernetesRuntimeDefinitionValues {
	return AwsEksKubernetesRuntimeDefinitionValues{
		Name:                         util.Ptr("eks-remote"),
		ZoneCount:                    util.Ptr(2),
		DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
		DefaultNodeGroupInitialSize:  util.Ptr(1),
		DefaultNodeGroupMinimumSize:  util.Ptr(1),
		DefaultNodeGroupMaximumSize:  util.Ptr(3),
		SupportType:                  supportType,
	}
}

// eksDefinitionAPI returns an API stub that creates kubernetes runtime and
// EKS definitions, and the EKS definition it was sent.
func eksDefinitionAPI(t *testing.T) (*machinetest.APIStub, *api_v0.AwsEksKubernetesRuntimeDefinition) {
	t.Helper()

	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{api_v0.KubernetesRuntimeDefinition{
			Common: api_v0.Common{ID: util.Ptr(uint(2)), CreatedAt: util.Ptr(time.Now())},
		}})
	})
	sent := &api_v0.AwsEksKubernetesRuntimeDefinition{}
	api.Mux.HandleFunc(api_v0.PathAwsEksKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(sent))
		created := *sent
		created.ID = util.Ptr(uint(3))
		created.CreatedAt = util.Ptr(time.Now())
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{created})
	})

	return api, sent
}

// TestAnEksConfigWithAnUnsupportedSupportTypeIsRefusedBeforeItIsSent covers
// #796's support type in an EKS config file.
func TestAnEksConfigWithAnUnsupportedSupportTypeIsRefusedBeforeItIsSent(t *testing.T) {
	config := AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: eksDefinitionValues(util.Ptr("extended"))}
	assert.ErrorContains(t, config.Validate(), `SupportType "extended" is not supported: use one of STANDARD, EXTENDED`)

	config.AwsEksKubernetesRuntimeDefinition = eksDefinitionValues(util.Ptr("EXTENDED"))
	assert.NoError(t, config.Validate())
	config.AwsEksKubernetesRuntimeDefinition = eksDefinitionValues(nil)
	assert.NoError(t, config.Validate())
}

// TestAnEksDefinitionConfigRecordsTheSupportTypeItsClustersGet covers an EKS
// definition created from config: it records the support type asked for,
// and the default when it's left out, and reports it back.
func TestAnEksDefinitionConfigRecordsTheSupportTypeItsClustersGet(t *testing.T) {
	for _, c := range []struct {
		asked *string
		want  string
	}{
		{util.Ptr("EXTENDED"), "EXTENDED"},
		{nil, "STANDARD"},
	} {
		api, sent := eksDefinitionAPI(t)
		config := &AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: eksDefinitionValues(c.asked)}
		created, err := config.Create(api.Client, api.Addr)
		require.NoError(t, err)
		assert.Equal(t, util.Ptr(c.want), sent.SupportType)
		assert.Equal(t, util.Ptr(c.want), created.AwsEksKubernetesRuntimeDefinition.SupportType)
	}
}

// TestAnEksRuntimeConfigSendsItsSupportType covers an AwsEksKubernetesRuntime
// config file, which creates the definition and the instance together: the
// definition carries its support type.
func TestAnEksRuntimeConfigSendsItsSupportType(t *testing.T) {
	config := AwsEksKubernetesRuntimeConfig{AwsEksKubernetesRuntime: AwsEksKubernetesRuntimeValues{
		Name:                         util.Ptr("eks-remote"),
		ZoneCount:                    util.Ptr(2),
		DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
		DefaultNodeGroupInitialSize:  util.Ptr(1),
		DefaultNodeGroupMinimumSize:  util.Ptr(1),
		DefaultNodeGroupMaximumSize:  util.Ptr(3),
		SupportType:                  util.Ptr("EXTENDED"),
	}}
	api, sent := eksDefinitionAPI(t)
	operations, definitions, _ := config.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())
	assert.Equal(t, util.Ptr("EXTENDED"), (*definitions)[0].AwsEksKubernetesRuntimeDefinition.SupportType)
	assert.Equal(t, util.Ptr("EXTENDED"), sent.SupportType)
}

// TestReplacingAnEksDefinitionKeepsItsSupportType covers a replace from a
// config that leaves the support type out: it sends the stored one, which
// the API refuses to change, rather than clearing it.
func TestReplacingAnEksDefinitionKeepsItsSupportType(t *testing.T) {
	name := "eks-remote"
	stored := api_v0.AwsEksKubernetesRuntimeDefinition{
		Common:      api_v0.Common{ID: util.Ptr(uint(1))},
		Definition:  api_v0.Definition{Name: &name},
		SupportType: util.Ptr("EXTENDED"),
	}
	replace := func(values AwsEksKubernetesRuntimeDefinitionValues) api_v0.AwsEksKubernetesRuntimeDefinition {
		api := &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathAwsEksKubernetesRuntimeDefinitions: stored}}
		config := &AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: values}
		_, err := config.Replace(&http.Client{}, api.serve(), name)
		require.NoError(t, err)
		var sent api_v0.AwsEksKubernetesRuntimeDefinition
		api.decodeSent(&sent)
		return sent
	}

	assert.Equal(t, util.Ptr("EXTENDED"), replace(eksDefinitionValues(nil)).SupportType)
	assert.Equal(t, util.Ptr("STANDARD"), replace(eksDefinitionValues(util.Ptr("STANDARD"))).SupportType,
		"one the config gives is sent, for the API to refuse")
}
