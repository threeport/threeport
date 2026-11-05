package v0

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAllowedCidrsRoundTripBetweenConfigAndTheApi covers how a config's
// list of CIDR blocks becomes the JSON array the API stores and back.
func TestAllowedCidrsRoundTripBetweenConfigAndTheApi(t *testing.T) {
	assert.Nil(t, cidrsToJSON(nil), "an unset list is not sent")

	empty := cidrsToJSON(&[]string{})
	require.NotNil(t, empty, "an empty list is sent, so that a replace can clear one")
	assert.JSONEq(t, `[]`, string(*empty))

	cidrs := []string{"203.0.113.0/24", "198.51.100.7/32"}
	stored := cidrsToJSON(&cidrs)
	require.NotNil(t, stored)
	assert.JSONEq(t, `["203.0.113.0/24","198.51.100.7/32"]`, string(*stored))
	assert.Equal(t, &cidrs, cidrsFromJSON(stored))

	assert.Nil(t, cidrsFromJSON(nil))
	assert.Nil(t, cidrsFromJSON(util.Ptr(datatypes.JSON(`null`))))
	assert.Nil(t, cidrsFromJSON(util.Ptr(datatypes.JSON(`not json`))))
}

// TestAConfigWithABadCidrBlockIsRefusedBeforeItIsSent covers the config
// validation that runs before any call to the API.
func TestAConfigWithABadCidrBlockIsRefusedBeforeItIsSent(t *testing.T) {
	bad := &[]string{"203.0.113.7/24"}
	good := &[]string{"203.0.113.0/24"}

	definition := KubernetesRuntimeDefinitionConfig{KubernetesRuntimeDefinition: KubernetesRuntimeDefinitionValues{
		Name: util.Ptr("remote"), InfraProvider: util.Ptr("eks"), AllowedCidrs: bad,
	}}
	err := definition.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `AllowedCidrs entry "203.0.113.7/24" has host bits set`)

	definition.KubernetesRuntimeDefinition.AllowedCidrs = good
	assert.NoError(t, definition.Validate())

	oke := OciOkeKubernetesRuntimeDefinitionConfig{OciOkeKubernetesRuntimeDefinition: OciOkeKubernetesRuntimeDefinitionValues{
		Name: util.Ptr("remote"), OciProviderName: util.Ptr("default"), WorkerNodeShape: util.Ptr("VM.Standard.E4.Flex"),
		WorkerNodeInitialCount: util.Ptr(2), AllowedCidrs: &[]string{"2001:db8::/32"},
	}}
	err = oke.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not an IPv4 CIDR block")
}

// TestAKubernetesRuntimeConfigSendsItsKubernetesApiAccess covers a
// KubernetesRuntime config file setting both fields: the definition it
// creates carries them.
func TestAKubernetesRuntimeConfigSendsItsKubernetesApiAccess(t *testing.T) {
	api := machinetest.NewAPIStub(t)
	var sent api_v0.KubernetesRuntimeDefinition
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		created := sent
		created.ID = util.Ptr(uint(3))
		created.CreatedAt = util.Ptr(time.Now())
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{created})
	})

	config := KubernetesRuntimeConfig{KubernetesRuntime: KubernetesRuntimeValues{
		Name:          util.Ptr("remote"),
		InfraProvider: util.Ptr("gke"),
		PrivateApi:    util.Ptr(true),
		AllowedCidrs:  &[]string{"10.8.0.0/16"},
	}}
	operations, definitions, _ := config.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())

	require.NotNil(t, sent.PrivateApi)
	assert.True(t, *sent.PrivateApi)
	require.NotNil(t, sent.AllowedCidrs)
	assert.JSONEq(t, `["10.8.0.0/16"]`, string(*sent.AllowedCidrs))

	require.Len(t, *definitions, 1)
	created := (*definitions)[0].KubernetesRuntimeDefinition
	assert.Equal(t, util.Ptr(true), created.PrivateApi)
	assert.Equal(t, &[]string{"10.8.0.0/16"}, created.AllowedCidrs)
}

// okeAPI stubs the endpoints an OKE definition config calls and records
// what each one is sent.
type okeAPI struct {
	mu            sync.Mutex
	krdCreated    *api_v0.KubernetesRuntimeDefinition
	krdPatched    map[string]json.RawMessage
	okeCreated    *api_v0.OciOkeKubernetesRuntimeDefinition
	okeReplaced   *api_v0.OciOkeKubernetesRuntimeDefinition
	krdPatchedIDs []string
}

func newOkeAPI(t *testing.T) (*machinetest.APIStub, *okeAPI) {
	t.Helper()
	api := machinetest.NewAPIStub(t)
	rec := &okeAPI{}
	krdID := uint(11)
	existing := api_v0.OciOkeKubernetesRuntimeDefinition{
		Common:                        api_v0.Common{ID: util.Ptr(uint(21)), CreatedAt: util.Ptr(time.Now())},
		Definition:                    api_v0.Definition{Name: util.Ptr("oke-remote")},
		WorkerNodeShape:               util.Ptr("VM.Standard.E4.Flex"),
		WorkerNodeInitialCount:        util.Ptr(int32(2)),
		KubernetesRuntimeDefinitionID: &krdID,
	}

	api.Mux.HandleFunc(api_v0.PathOciProviders, func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{
			api_v0.OciProvider{Common: api_v0.Common{ID: util.Ptr(uint(1))}, Name: util.Ptr("default")},
		})
	})
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		var sent api_v0.KubernetesRuntimeDefinition
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		rec.mu.Lock()
		rec.krdCreated = &sent
		rec.mu.Unlock()
		sent.ID = &krdID
		sent.CreatedAt = util.Ptr(time.Now())
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent})
	})
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions+"/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var sent map[string]json.RawMessage
		assert.NoError(t, json.Unmarshal(body, &sent))
		rec.mu.Lock()
		rec.krdPatched = sent
		rec.krdPatchedIDs = append(rec.krdPatchedIDs, r.URL.Path)
		rec.mu.Unlock()
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{
			api_v0.KubernetesRuntimeDefinition{Common: api_v0.Common{ID: &krdID}},
		})
	})
	api.Mux.HandleFunc(api_v0.PathOciOkeKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{existing})
		case http.MethodPost:
			var sent api_v0.OciOkeKubernetesRuntimeDefinition
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			rec.mu.Lock()
			rec.okeCreated = &sent
			rec.mu.Unlock()
			sent.ID = existing.ID
			sent.CreatedAt = existing.CreatedAt
			machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent})
		}
	})
	api.Mux.HandleFunc(api_v0.PathOciOkeKubernetesRuntimeDefinitions+"/", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		var sent api_v0.OciOkeKubernetesRuntimeDefinition
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		rec.mu.Lock()
		rec.okeReplaced = &sent
		rec.mu.Unlock()
		sent.ID = existing.ID
		sent.CreatedAt = existing.CreatedAt
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{sent})
	})

	return api, rec
}

// okeDefinitionConfig returns an OKE definition config with the given
// Kubernetes API access.
func okeDefinitionConfig(privateApi *bool, allowedCidrs *[]string) *OciOkeKubernetesRuntimeDefinitionConfig {
	return &OciOkeKubernetesRuntimeDefinitionConfig{OciOkeKubernetesRuntimeDefinition: OciOkeKubernetesRuntimeDefinitionValues{
		Name:                   util.Ptr("oke-remote"),
		OciProviderName:        util.Ptr("default"),
		WorkerNodeShape:        util.Ptr("VM.Standard.E4.Flex"),
		WorkerNodeInitialCount: util.Ptr(2),
		PrivateApi:             privateApi,
		AllowedCidrs:           allowedCidrs,
	}}
}

// TestAnOkeDefinitionSetsItsKubernetesApiAccessOnBothDefinitions covers the
// OKE config, which creates the kubernetes runtime definition and the OKE
// definition itself rather than through the definition reconciler: both
// carry the fields on create.
func TestAnOkeDefinitionSetsItsKubernetesApiAccessOnBothDefinitions(t *testing.T) {
	api, rec := newOkeAPI(t)

	created, err := okeDefinitionConfig(util.Ptr(true), &[]string{"10.8.0.0/16"}).Create(api.Client, api.Addr)
	require.NoError(t, err)

	require.NotNil(t, rec.krdCreated)
	assert.Equal(t, util.Ptr(true), rec.krdCreated.PrivateApi)
	require.NotNil(t, rec.krdCreated.AllowedCidrs)
	assert.JSONEq(t, `["10.8.0.0/16"]`, string(*rec.krdCreated.AllowedCidrs))

	require.NotNil(t, rec.okeCreated)
	assert.Equal(t, util.Ptr(true), rec.okeCreated.PrivateApi)
	require.NotNil(t, rec.okeCreated.AllowedCidrs)
	assert.JSONEq(t, `["10.8.0.0/16"]`, string(*rec.okeCreated.AllowedCidrs))

	assert.Equal(t, util.Ptr(true), created.OciOkeKubernetesRuntimeDefinition.PrivateApi)
	assert.Equal(t, &[]string{"10.8.0.0/16"}, created.OciOkeKubernetesRuntimeDefinition.AllowedCidrs)
}

// TestReplacingAnOkeDefinitionKeepsItsKubernetesRuntimeDefinitionInStep
// covers a replace of the OKE definition: the kubernetes runtime definition
// it belongs to is patched to the same values, and a replace that leaves
// the fields out clears them there too instead of leaving the old ones.
func TestReplacingAnOkeDefinitionKeepsItsKubernetesRuntimeDefinitionInStep(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		api, rec := newOkeAPI(t)
		_, err := okeDefinitionConfig(util.Ptr(true), &[]string{"10.8.0.0/16"}).Replace(api.Client, api.Addr, "oke-remote")
		require.NoError(t, err)

		require.NotNil(t, rec.okeReplaced)
		assert.Equal(t, util.Ptr(true), rec.okeReplaced.PrivateApi)
		assert.Equal(t, []string{"/v0/kubernetes-runtime-definitions/11"}, rec.krdPatchedIDs)
		assert.JSONEq(t, `true`, string(rec.krdPatched["PrivateApi"]))
		assert.JSONEq(t, `["10.8.0.0/16"]`, string(rec.krdPatched["AllowedCidrs"]))
	})
	t.Run("left out", func(t *testing.T) {
		api, rec := newOkeAPI(t)
		_, err := okeDefinitionConfig(nil, nil).Replace(api.Client, api.Addr, "oke-remote")
		require.NoError(t, err)

		assert.Equal(t, []string{"/v0/kubernetes-runtime-definitions/11"}, rec.krdPatchedIDs)
		assert.JSONEq(t, `false`, string(rec.krdPatched["PrivateApi"]))
		assert.JSONEq(t, `[]`, string(rec.krdPatched["AllowedCidrs"]))
	})
}
