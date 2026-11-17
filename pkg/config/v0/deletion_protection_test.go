package v0

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestEksAndGkeDefinitionConfigsTurnDeletionProtectionOnAndOff covers the
// EKS and GKE definition configs, through which a protected cluster's
// protection is turned off: a create sends DeletionProtection to both
// definitions, a replace that leaves it out keeps the stored value, so a
// protected definition isn't unprotected by a config that doesn't mention
// it, and a replace with false turns it off.
func TestEksAndGkeDefinitionConfigsTurnDeletionProtectionOnAndOff(t *testing.T) {
	type sentDefinitions struct{ runtime, provider, replaced map[string]any }
	serve := func(t *testing.T, providerPath string) (*machinetest.APIStub, *sentDefinitions) {
		api := machinetest.NewAPIStub(t)
		sent := &sentDefinitions{}
		decode := func(r *http.Request) map[string]any {
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			body["ID"] = 1
			return body
		}
		stored := map[string]any{"ID": 1, "Name": "remote", "DeletionProtection": true}
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

	for name, c := range map[string]struct {
		path   string
		create func(api *machinetest.APIStub, protection *bool) *bool
		update func(api *machinetest.APIStub, protection *bool)
	}{
		"EKS": {
			path: api_v0.PathAwsEksKubernetesRuntimeDefinitions,
			create: func(api *machinetest.APIStub, protection *bool) *bool {
				created, err := eksDefinitionWithProtection(protection).Create(api.Client, api.Addr)
				require.NoError(t, err)
				return created.AwsEksKubernetesRuntimeDefinition.DeletionProtection
			},
			update: func(api *machinetest.APIStub, protection *bool) {
				_, err := eksDefinitionWithProtection(protection).Replace(api.Client, api.Addr, "remote")
				require.NoError(t, err)
			},
		},
		"GKE": {
			path: api_v0.PathGcpGkeKubernetesRuntimeDefinitions,
			create: func(api *machinetest.APIStub, protection *bool) *bool {
				created, err := gkeDefinitionWithProtection(protection).Create(api.Client, api.Addr)
				require.NoError(t, err)
				return created.GcpGkeKubernetesRuntimeDefinition.DeletionProtection
			},
			update: func(api *machinetest.APIStub, protection *bool) {
				_, err := gkeDefinitionWithProtection(protection).Replace(api.Client, api.Addr, "remote")
				require.NoError(t, err)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			api, sent := serve(t, c.path)
			assert.Equal(t, util.Ptr(true), c.create(api, util.Ptr(true)))
			assert.Equal(t, true, sent.runtime["DeletionProtection"])
			assert.Equal(t, true, sent.provider["DeletionProtection"])

			c.update(api, nil)
			assert.Equal(t, true, sent.replaced["DeletionProtection"], "a replace without it keeps the stored protection")

			c.update(api, util.Ptr(false))
			assert.Equal(t, false, sent.replaced["DeletionProtection"], "a replace with false turns it off")
		})
	}
}

// TestAnOkeDefinitionConfigTurnsDeletionProtectionOnAndOff covers the OKE
// definition config: a create sends DeletionProtection to both definitions,
// and a replace with false turns it off.
func TestAnOkeDefinitionConfigTurnsDeletionProtectionOnAndOff(t *testing.T) {
	api, rec := newOkeAPI(t)
	config := okeDefinitionConfig(nil, nil)
	config.OciOkeKubernetesRuntimeDefinition.DeletionProtection = util.Ptr(true)
	created, err := config.Create(api.Client, api.Addr)
	require.NoError(t, err)
	assert.Equal(t, util.Ptr(true), rec.krdCreated.DeletionProtection)
	assert.Equal(t, util.Ptr(true), rec.okeCreated.DeletionProtection)
	assert.Equal(t, util.Ptr(true), created.OciOkeKubernetesRuntimeDefinition.DeletionProtection)

	api, rec = newOkeAPI(t)
	config = okeDefinitionConfig(nil, nil)
	config.OciOkeKubernetesRuntimeDefinition.DeletionProtection = util.Ptr(false)
	_, err = config.Replace(api.Client, api.Addr, "oke-remote")
	require.NoError(t, err)
	require.NotNil(t, rec.okeReplaced)
	assert.Equal(t, util.Ptr(false), rec.okeReplaced.DeletionProtection)
}

// TestRuntimeConfigsSendTheirDeletionProtection covers the config files that
// create a definition and an instance together: the definition they create
// is protected as the config asks.
func TestRuntimeConfigsSendTheirDeletionProtection(t *testing.T) {
	for name, c := range map[string]struct {
		path   string
		create func(api *machinetest.APIStub) error
	}{
		"KubernetesRuntime": {api_v0.PathKubernetesRuntimeDefinitions, func(api *machinetest.APIStub) error {
			config := KubernetesRuntimeConfig{KubernetesRuntime: KubernetesRuntimeValues{
				Name:               util.Ptr("remote"),
				InfraProvider:      util.Ptr(api_v0.KubernetesRuntimeInfraProviderGKE),
				DeletionProtection: util.Ptr(true),
			}}
			operations, _, _ := config.GetOperations(api.Client, api.Addr)
			return operations.Operations[0].Create()
		}},
		"AwsEksKubernetesRuntime": {api_v0.PathAwsEksKubernetesRuntimeDefinitions, func(api *machinetest.APIStub) error {
			values := eksDefinitionWithProtection(nil).AwsEksKubernetesRuntimeDefinition
			config := AwsEksKubernetesRuntimeConfig{AwsEksKubernetesRuntime: AwsEksKubernetesRuntimeValues{
				Name: values.Name, ZoneCount: values.ZoneCount, DefaultNodeGroupInstanceType: values.DefaultNodeGroupInstanceType,
				DefaultNodeGroupInitialSize: values.DefaultNodeGroupInitialSize, DefaultNodeGroupMinimumSize: values.DefaultNodeGroupMinimumSize,
				DefaultNodeGroupMaximumSize: values.DefaultNodeGroupMaximumSize, DeletionProtection: util.Ptr(true),
			}}
			operations, _, _ := config.GetOperations(api.Client, api.Addr)
			return operations.Operations[0].Create()
		}},
		"GcpGkeKubernetesRuntime": {api_v0.PathGcpGkeKubernetesRuntimeDefinitions, func(api *machinetest.APIStub) error {
			values := gkeDefinitionWithProtection(nil).GcpGkeKubernetesRuntimeDefinition
			config := GcpGkeKubernetesRuntimeConfig{GcpGkeKubernetesRuntime: GcpGkeKubernetesRuntimeValues{
				Name: values.Name, ZoneCount: values.ZoneCount, DefaultNodeGroupInstanceType: values.DefaultNodeGroupInstanceType,
				DefaultNodeGroupInitialSize: values.DefaultNodeGroupInitialSize, DefaultNodeGroupMinimumSize: values.DefaultNodeGroupMinimumSize,
				DefaultNodeGroupMaximumSize: values.DefaultNodeGroupMaximumSize, DeletionProtection: util.Ptr(true),
			}}
			operations, _, _ := config.GetOperations(api.Client, api.Addr)
			return operations.Operations[0].Create()
		}},
	} {
		t.Run(name, func(t *testing.T) {
			api := machinetest.NewAPIStub(t)
			sent := map[string]map[string]any{}
			record := func(path string) {
				api.Mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					sent[path] = body
					body["ID"] = 1
					machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{body})
				})
			}
			record(api_v0.PathKubernetesRuntimeDefinitions)
			if c.path != api_v0.PathKubernetesRuntimeDefinitions {
				record(c.path)
			}

			require.NoError(t, c.create(api))
			assert.Equal(t, true, sent[c.path]["DeletionProtection"])
		})
	}
}

// eksDefinitionWithProtection returns an EKS definition config with
// DeletionProtection as given.
func eksDefinitionWithProtection(protection *bool) *AwsEksKubernetesRuntimeDefinitionConfig {
	return &AwsEksKubernetesRuntimeDefinitionConfig{AwsEksKubernetesRuntimeDefinition: AwsEksKubernetesRuntimeDefinitionValues{
		Name: util.Ptr("remote"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("t3.medium"),
		DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		DeletionProtection: protection,
	}}
}

// gkeDefinitionWithProtection returns a GKE definition config with
// DeletionProtection as given.
func gkeDefinitionWithProtection(protection *bool) *GcpGkeKubernetesRuntimeDefinitionConfig {
	return &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: GcpGkeKubernetesRuntimeDefinitionValues{
		Name: util.Ptr("remote"), ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
		DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
		DeletionProtection: protection,
	}}
}
