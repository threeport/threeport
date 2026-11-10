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

// TestAConfigWithAnUnsupportedKubernetesVersionIsRefusedBeforeItIsSent
// covers the config files that set a version.
func TestAConfigWithAnUnsupportedKubernetesVersionIsRefusedBeforeItIsSent(t *testing.T) {
	definition := KubernetesRuntimeDefinitionConfig{KubernetesRuntimeDefinition: KubernetesRuntimeDefinitionValues{
		Name:              util.Ptr("remote"),
		InfraProvider:     util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
		KubernetesVersion: util.Ptr("1.32"),
	}}
	assert.ErrorContains(t, definition.Validate(), `KubernetesVersion "1.32" is not supported`)
	definition.KubernetesRuntimeDefinition.KubernetesVersion = util.Ptr("1.35")
	assert.NoError(t, definition.Validate())

	oke := okeDefinitionConfig(nil, nil)
	oke.OciOkeKubernetesRuntimeDefinition.KubernetesVersion = util.Ptr("v1.36.4")
	assert.ErrorContains(t, oke.Validate(), `KubernetesVersion "v1.36.4" is not supported`)
}

// TestAKubernetesRuntimeConfigSendsItsKubernetesVersion covers a
// KubernetesRuntime config file setting the version: the definition it
// creates carries it.
func TestAKubernetesRuntimeConfigSendsItsKubernetesVersion(t *testing.T) {
	api := machinetest.NewAPIStub(t)
	var sent api_v0.KubernetesRuntimeDefinition
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		created := sent
		created.ID = util.Ptr(uint(3))
		created.CreatedAt = util.Ptr(time.Now())
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{created})
	})

	config := KubernetesRuntimeConfig{KubernetesRuntime: KubernetesRuntimeValues{
		Name:              util.Ptr("remote"),
		InfraProvider:     util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
		KubernetesVersion: util.Ptr("1.35"),
	}}
	operations, definitions, _ := config.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())

	assert.Equal(t, util.Ptr("1.35"), sent.KubernetesVersion)
	assert.Equal(t, util.Ptr("1.35"), (*definitions)[0].KubernetesRuntimeDefinition.KubernetesVersion)
}

// TestReplacingADefinitionKeepsItsKubernetesVersion covers a replace from a
// config that leaves the version out: it sends the stored version, which the
// API refuses to change, rather than clearing it.
func TestReplacingADefinitionKeepsItsKubernetesVersion(t *testing.T) {
	name := "remote"
	stored := api_v0.KubernetesRuntimeDefinition{
		Common:            api_v0.Common{ID: util.Ptr(uint(1))},
		Definition:        api_v0.Definition{Name: &name},
		InfraProvider:     util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
		KubernetesVersion: util.Ptr("1.35"),
	}
	replace := func(version *string) api_v0.KubernetesRuntimeDefinition {
		api := &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathKubernetesRuntimeDefinitions: stored}}
		config := &KubernetesRuntimeDefinitionConfig{KubernetesRuntimeDefinition: KubernetesRuntimeDefinitionValues{
			Name:              &name,
			InfraProvider:     util.Ptr(api_v0.KubernetesRuntimeInfraProviderEKS),
			KubernetesVersion: version,
		}}
		_, err := config.Replace(&http.Client{}, api.serve(), name)
		require.NoError(t, err)
		var sent api_v0.KubernetesRuntimeDefinition
		api.decodeSent(&sent)
		return sent
	}

	assert.Equal(t, util.Ptr("1.35"), replace(nil).KubernetesVersion)
	assert.Equal(t, util.Ptr("1.36"), replace(util.Ptr("1.36")).KubernetesVersion,
		"a version the config gives is sent, for the API to refuse")
}

// TestAnOkeDefinitionRecordsTheVersionItsClustersRun covers the OKE config,
// which creates both definitions itself: the kubernetes runtime definition
// gets the version asked for, and the OKE definition the version its
// clusters are created on, the default when none is asked for.
func TestAnOkeDefinitionRecordsTheVersionItsClustersRun(t *testing.T) {
	t.Run("asked for", func(t *testing.T) {
		api, rec := newOkeAPI(t)
		config := okeDefinitionConfig(nil, nil)
		config.OciOkeKubernetesRuntimeDefinition.KubernetesVersion = util.Ptr("1.35")
		created, err := config.Create(api.Client, api.Addr)
		require.NoError(t, err)

		assert.Equal(t, util.Ptr("1.35"), rec.krdCreated.KubernetesVersion)
		assert.Equal(t, util.Ptr("1.35"), rec.okeCreated.KubernetesVersion)
		assert.Equal(t, util.Ptr("1.35"), created.OciOkeKubernetesRuntimeDefinition.KubernetesVersion)
	})
	t.Run("left out", func(t *testing.T) {
		api, rec := newOkeAPI(t)
		_, err := okeDefinitionConfig(nil, nil).Create(api.Client, api.Addr)
		require.NoError(t, err)

		assert.Nil(t, rec.krdCreated.KubernetesVersion)
		assert.Equal(t, util.Ptr(api_v0.DefaultKubernetesVersion), rec.okeCreated.KubernetesVersion)
	})
	t.Run("replaced without it", func(t *testing.T) {
		api, rec := newOkeAPI(t)
		_, err := okeDefinitionConfig(nil, nil).Replace(api.Client, api.Addr, "oke-remote")
		require.NoError(t, err)

		require.NotNil(t, rec.okeReplaced)
		assert.Nil(t, rec.okeReplaced.KubernetesVersion, "the stored OKE definition sets none, and the replace keeps that")
		assert.NotContains(t, rec.krdPatched, "KubernetesVersion")
	})
}
