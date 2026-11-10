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

// providerDefinitionAPI returns an API stub that creates kubernetes runtime
// definitions and the provider definitions at path, and records the bodies
// of both.
func providerDefinitionAPI(t *testing.T, path string) (*machinetest.APIStub, *map[string]any, *map[string]any) {
	t.Helper()

	api := machinetest.NewAPIStub(t)
	krd, provider := map[string]any{}, map[string]any{}
	record := func(sent *map[string]any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			assert.NoError(t, json.NewDecoder(r.Body).Decode(sent))
			created := map[string]any{}
			for key, value := range *sent {
				created[key] = value
			}
			created["ID"] = 3
			created["CreatedAt"] = time.Now()
			machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{created})
		}
	}
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, record(&krd))
	api.Mux.HandleFunc(path, record(&provider))

	return api, &krd, &provider
}

// TestTheEksAndGkeConfigsTakeAKubernetesVersion covers the EKS and GKE
// definition configs, which create the kubernetes runtime definition already
// reconciled, so the reconciler never records a version on theirs: the
// kubernetes runtime definition gets the version asked for, the provider
// definition the version its clusters are created on, the default when none
// is asked for, and an unsupported one is refused before it is sent.
func TestTheEksAndGkeConfigsTakeAKubernetesVersion(t *testing.T) {
	eks := func(version *string) *AwsEksKubernetesRuntimeDefinitionConfig {
		return &AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: AwsEksKubernetesRuntimeDefinitionValues{
			Name: util.Ptr("eks-remote"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
			DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesVersion: version,
		}}
	}
	gke := func(version *string) *GcpGkeKubernetesRuntimeDefinitionConfig {
		values := gkeDefinitionValues(nil, nil)
		values.KubernetesVersion = version
		return &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: values}
	}

	assert.ErrorContains(t, eks(util.Ptr("1.32")).Validate(), `KubernetesVersion "1.32" is not supported`)
	assert.ErrorContains(t, gke(util.Ptr("1.36.4")).Validate(), `KubernetesVersion "1.36.4" is not supported`)

	for name, c := range map[string]struct {
		path   string
		create func(api *machinetest.APIStub, version *string) *string
	}{
		"EKS": {api_v0.PathAwsEksKubernetesRuntimeDefinitions, func(api *machinetest.APIStub, version *string) *string {
			created, err := eks(version).Create(api.Client, api.Addr)
			require.NoError(t, err)
			return created.AwsEksKubernetesRuntimeDefinition.KubernetesVersion
		}},
		"GKE": {api_v0.PathGcpGkeKubernetesRuntimeDefinitions, func(api *machinetest.APIStub, version *string) *string {
			created, err := gke(version).Create(api.Client, api.Addr)
			require.NoError(t, err)
			return created.GcpGkeKubernetesRuntimeDefinition.KubernetesVersion
		}},
	} {
		t.Run(name, func(t *testing.T) {
			api, krd, provider := providerDefinitionAPI(t, c.path)
			assert.Equal(t, util.Ptr("1.35"), c.create(api, util.Ptr("1.35")))
			assert.Equal(t, "1.35", (*krd)["KubernetesVersion"])
			assert.Equal(t, "1.35", (*provider)["KubernetesVersion"])

			api, krd, provider = providerDefinitionAPI(t, c.path)
			assert.Equal(t, util.Ptr(api_v0.DefaultKubernetesVersion), c.create(api, nil))
			assert.Nil(t, (*krd)["KubernetesVersion"], "the kubernetes runtime definition keeps what was asked for")
			assert.Equal(t, api_v0.DefaultKubernetesVersion, (*provider)["KubernetesVersion"])
		})
	}
}

// TestTheEksAndGkeRuntimeConfigsSendTheirKubernetesVersion covers the
// AwsEksKubernetesRuntime and GcpGkeKubernetesRuntime configs, which create
// the definition and the instance together: the definition carries the
// version.
func TestTheEksAndGkeRuntimeConfigsSendTheirKubernetesVersion(t *testing.T) {
	api, _, provider := providerDefinitionAPI(t, api_v0.PathAwsEksKubernetesRuntimeDefinitions)
	eks := AwsEksKubernetesRuntimeConfig{AwsEksKubernetesRuntime: AwsEksKubernetesRuntimeValues{
		Name: util.Ptr("eks-remote"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
		DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		KubernetesVersion: util.Ptr("1.35"),
	}}
	operations, _, _ := eks.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())
	assert.Equal(t, "1.35", (*provider)["KubernetesVersion"])

	api, _, provider = providerDefinitionAPI(t, api_v0.PathGcpGkeKubernetesRuntimeDefinitions)
	gke := GcpGkeKubernetesRuntimeConfig{GcpGkeKubernetesRuntime: GcpGkeKubernetesRuntimeValues{
		Name: util.Ptr("gke-remote"), ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
		DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		KubernetesVersion: util.Ptr("1.35"),
	}}
	operations, _, _ = gke.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())
	assert.Equal(t, "1.35", (*provider)["KubernetesVersion"])
}

// TestReplacingAnEksOrGkeDefinitionKeepsItsKubernetesVersion covers a
// replace from an EKS or GKE config that leaves the version out: it sends
// the stored version, which the API refuses to change, rather than clearing
// it.
func TestReplacingAnEksOrGkeDefinitionKeepsItsKubernetesVersion(t *testing.T) {
	eksStored := api_v0.AwsEksKubernetesRuntimeDefinition{
		Common: api_v0.Common{ID: util.Ptr(uint(1))}, Definition: api_v0.Definition{Name: util.Ptr("eks-remote")},
		KubernetesVersion: util.Ptr("1.35"),
	}
	api := &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathAwsEksKubernetesRuntimeDefinitions: eksStored}}
	eks := &AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: AwsEksKubernetesRuntimeDefinitionValues{
		Name: util.Ptr("eks-remote"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
		DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
	}}
	_, err := eks.Replace(&http.Client{}, api.serve(), "eks-remote")
	require.NoError(t, err)
	var eksSent api_v0.AwsEksKubernetesRuntimeDefinition
	api.decodeSent(&eksSent)
	assert.Equal(t, util.Ptr("1.35"), eksSent.KubernetesVersion)

	gkeStored := api_v0.GcpGkeKubernetesRuntimeDefinition{
		Common: api_v0.Common{ID: util.Ptr(uint(1))}, Definition: api_v0.Definition{Name: util.Ptr("gke-remote")},
		KubernetesVersion: util.Ptr("1.35"),
	}
	api = &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathGcpGkeKubernetesRuntimeDefinitions: gkeStored}}
	gke := &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: gkeDefinitionValues(nil, nil)}
	_, err = gke.Replace(&http.Client{}, api.serve(), "gke-remote")
	require.NoError(t, err)
	var gkeSent api_v0.GcpGkeKubernetesRuntimeDefinition
	api.decodeSent(&gkeSent)
	assert.Equal(t, util.Ptr("1.35"), gkeSent.KubernetesVersion)

	gke.GcpGkeKubernetesRuntimeDefinition.KubernetesVersion = util.Ptr("1.36")
	api = &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathGcpGkeKubernetesRuntimeDefinitions: gkeStored}}
	_, err = gke.Replace(&http.Client{}, api.serve(), "gke-remote")
	require.NoError(t, err)
	api.decodeSent(&gkeSent)
	assert.Equal(t, util.Ptr("1.36"), gkeSent.KubernetesVersion, "a version the config gives is sent, for the API to refuse")
}
