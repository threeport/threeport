package v0

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestEksAndGkeDefinitionsRecordTheVersionTheirClustersRun covers the EKS
// and GKE config files, which create both definitions themselves: a version
// the config gives is checked before it is sent, the kubernetes runtime
// definition gets it, and the provider definition the version its clusters
// are created on, the default when none is given.  A replace that leaves it
// out keeps the stored one.
func TestEksAndGkeDefinitionsRecordTheVersionTheirClustersRun(t *testing.T) {
	type sentDefinitions struct{ runtime, provider, replaced map[string]any }
	serve := func(t *testing.T, providerPath string, stored map[string]any) (*machinetest.APIStub, *sentDefinitions) {
		api := machinetest.NewAPIStub(t)
		sent := &sentDefinitions{}
		decode := func(r *http.Request) map[string]any {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			body["ID"] = 1
			return body
		}
		api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
			sent.runtime = decode(r)
			machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent.runtime})
		})
		api.Mux.HandleFunc(providerPath, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{stored})
				return
			}
			sent.provider = decode(r)
			machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent.provider})
		})
		api.Mux.HandleFunc(providerPath+"/", func(w http.ResponseWriter, r *http.Request) {
			sent.replaced = decode(r)
			machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{sent.replaced})
		})
		return api, sent
	}
	stored := map[string]any{"ID": 1, "Name": "remote", "KubernetesVersion": "1.35"}

	t.Run("EKS", func(t *testing.T) {
		config := func(version *string) *AwsEksKubernetesRuntimeDefinitionConfig {
			return &AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: AwsEksKubernetesRuntimeDefinitionValues{
				Name: util.Ptr("remote"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
				DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
				KubernetesVersion: version,
			}}
		}
		assert.ErrorContains(t, config(util.Ptr("1.32")).Validate(), `KubernetesVersion "1.32" is not supported`)

		api, sent := serve(t, api_v0.PathAwsEksKubernetesRuntimeDefinitions, stored)
		created, err := config(util.Ptr("1.35")).Create(api.Client, api.Addr)
		require.NoError(t, err)
		assert.Equal(t, "1.35", sent.runtime["KubernetesVersion"])
		assert.Equal(t, "1.35", sent.provider["KubernetesVersion"])
		assert.Equal(t, util.Ptr("1.35"), created.AwsEksKubernetesRuntimeDefinition.KubernetesVersion)

		_, err = config(nil).Create(api.Client, api.Addr)
		require.NoError(t, err)
		assert.NotContains(t, sent.runtime, "KubernetesVersion")
		assert.Equal(t, api_v0.DefaultKubernetesVersion, sent.provider["KubernetesVersion"])

		_, err = config(nil).Replace(api.Client, api.Addr, "remote")
		require.NoError(t, err)
		assert.Equal(t, "1.35", sent.replaced["KubernetesVersion"], "a replace without it keeps the stored one")
	})
	t.Run("GKE", func(t *testing.T) {
		config := func(version *string) *GcpGkeKubernetesRuntimeDefinitionConfig {
			return &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: GcpGkeKubernetesRuntimeDefinitionValues{
				Name: util.Ptr("remote"), ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
				DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
				KubernetesVersion: version,
			}}
		}
		assert.ErrorContains(t, config(util.Ptr("v1.36.4")).Validate(), `KubernetesVersion "v1.36.4" is not supported`)

		api, sent := serve(t, api_v0.PathGcpGkeKubernetesRuntimeDefinitions, stored)
		created, err := config(util.Ptr("1.36")).Create(api.Client, api.Addr)
		require.NoError(t, err)
		assert.Equal(t, "1.36", sent.runtime["KubernetesVersion"])
		assert.Equal(t, "1.36", sent.provider["KubernetesVersion"])
		assert.Equal(t, util.Ptr("1.36"), created.GcpGkeKubernetesRuntimeDefinition.KubernetesVersion)

		_, err = config(nil).Create(api.Client, api.Addr)
		require.NoError(t, err)
		assert.Equal(t, api_v0.DefaultKubernetesVersion, sent.provider["KubernetesVersion"])

		_, err = config(nil).Replace(api.Client, api.Addr, "remote")
		require.NoError(t, err)
		assert.Equal(t, "1.35", sent.replaced["KubernetesVersion"], "a replace without it keeps the stored one")
	})
}

// TestReplacingAnEksOrGkeDefinitionSendsItsVersion covers a replace from an
// EKS or GKE config: a new version is sent on the provider definition, whose
// update asks its clusters to be upgraded, and a config that leaves it out
// keeps the recorded one.  The kubernetes runtime definition, which keeps the
// version asked for at creation, isn't touched.
func TestReplacingAnEksOrGkeDefinitionSendsItsVersion(t *testing.T) {
	type sentRequests struct {
		replaced map[string]any
		patched  []map[string]any
		paths    []string
	}
	serve := func(t *testing.T, path string, stored any) (string, *sentRequests) {
		sent := &sentRequests{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var fields map[string]any
			if len(body) > 0 {
				require.NoError(t, json.Unmarshal(body, &fields))
			}
			object := any(fields)
			switch r.Method {
			case http.MethodGet:
				require.True(t, strings.HasPrefix(r.URL.Path, path), r.URL.Path)
				object = stored
			case http.MethodPut:
				sent.replaced = fields
				fields["CreatedAt"] = time.Now()
			case http.MethodPatch:
				sent.patched = append(sent.patched, fields)
				sent.paths = append(sent.paths, r.URL.Path)
			}
			respondWithObject(t, w, object)
		}))
		t.Cleanup(server.Close)
		return strings.TrimPrefix(server.URL, "http://"), sent
	}

	eksStored := api_v0.AwsEksKubernetesRuntimeDefinition{
		Common: api_v0.Common{ID: util.Ptr(uint(1))}, Definition: api_v0.Definition{Name: util.Ptr("eks-remote")},
		KubernetesVersion: util.Ptr("1.35"), KubernetesRuntimeDefinitionID: util.Ptr(uint(5)),
	}
	eks := func(version *string) *AwsEksKubernetesRuntimeDefinitionConfig {
		return &AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: AwsEksKubernetesRuntimeDefinitionValues{
			Name: util.Ptr("eks-remote"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
			DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesVersion: version,
		}}
	}
	gkeStored := api_v0.GcpGkeKubernetesRuntimeDefinition{
		Common: api_v0.Common{ID: util.Ptr(uint(1))}, Definition: api_v0.Definition{Name: util.Ptr("gke-remote")},
		KubernetesVersion: util.Ptr("1.35"), KubernetesRuntimeDefinitionID: util.Ptr(uint(5)),
	}
	gke := func(version *string) *GcpGkeKubernetesRuntimeDefinitionConfig {
		values := gkeDefinitionValues(nil, nil)
		values.KubernetesVersion = version
		return &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: values}
	}

	for name, replace := range map[string]func(version *string) *sentRequests{
		"EKS": func(version *string) *sentRequests {
			endpoint, sent := serve(t, api_v0.PathAwsEksKubernetesRuntimeDefinitions, eksStored)
			_, err := eks(version).Replace(&http.Client{}, endpoint, "eks-remote")
			require.NoError(t, err)
			return sent
		},
		"GKE": func(version *string) *sentRequests {
			endpoint, sent := serve(t, api_v0.PathGcpGkeKubernetesRuntimeDefinitions, gkeStored)
			_, err := gke(version).Replace(&http.Client{}, endpoint, "gke-remote")
			require.NoError(t, err)
			return sent
		},
	} {
		t.Run(name, func(t *testing.T) {
			sent := replace(nil)
			assert.Equal(t, "1.35", sent.replaced["KubernetesVersion"], "a config that leaves the version out keeps it")
			assert.Empty(t, sent.patched)

			sent = replace(util.Ptr("1.36"))
			assert.Equal(t, "1.36", sent.replaced["KubernetesVersion"])
			assert.Empty(t, sent.patched)
		})
	}
}
