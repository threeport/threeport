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

// gkeDefinitionValues returns the values of a GKE definition config with
// the given release channel and maintenance window days.
func gkeDefinitionValues(channel, days *string) GcpGkeKubernetesRuntimeDefinitionValues {
	return GcpGkeKubernetesRuntimeDefinitionValues{
		Name:                         util.Ptr("gke-remote"),
		ZoneCount:                    util.Ptr(3),
		DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
		DefaultNodeGroupInitialSize:  util.Ptr(1),
		DefaultNodeGroupMinimumSize:  util.Ptr(1),
		DefaultNodeGroupMaximumSize:  util.Ptr(3),
		ReleaseChannel:               channel,
		MaintenanceWindowDays:        days,
	}
}

// gkeDefinitionAPI returns an API stub that creates kubernetes runtime and
// GKE definitions, and the GKE definition it was sent.
func gkeDefinitionAPI(t *testing.T) (*machinetest.APIStub, *api_v0.GcpGkeKubernetesRuntimeDefinition) {
	t.Helper()

	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(api_v0.PathKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{api_v0.KubernetesRuntimeDefinition{
			Common: api_v0.Common{ID: util.Ptr(uint(2)), CreatedAt: util.Ptr(time.Now())},
		}})
	})
	sent := &api_v0.GcpGkeKubernetesRuntimeDefinition{}
	api.Mux.HandleFunc(api_v0.PathGcpGkeKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(sent))
		created := *sent
		created.ID = util.Ptr(uint(3))
		created.CreatedAt = util.Ptr(time.Now())
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{created})
	})

	return api, sent
}

// createGkeDefinition creates a GKE definition from config on an API stub,
// and returns the GKE definition it sent.
func createGkeDefinition(t *testing.T, config *GcpGkeKubernetesRuntimeDefinitionConfig) api_v0.GcpGkeKubernetesRuntimeDefinition {
	t.Helper()

	api, sent := gkeDefinitionAPI(t)
	_, err := config.Create(api.Client, api.Addr)
	require.NoError(t, err)
	return *sent
}

// TestAGkeConfigWithAnUnsupportedUpgradeSettingIsRefusedBeforeItIsSent
// covers the release channel and maintenance window in a GKE config file.
func TestAGkeConfigWithAnUnsupportedUpgradeSettingIsRefusedBeforeItIsSent(t *testing.T) {
	config := GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: gkeDefinitionValues(util.Ptr("RAPID"), nil)}
	assert.ErrorContains(t, config.Validate(), `ReleaseChannel "RAPID" is not supported`)

	config.GcpGkeKubernetesRuntimeDefinition = gkeDefinitionValues(nil, util.Ptr("SAT"))
	assert.ErrorContains(t, config.Validate(), `MaintenanceWindowDays "SAT" is not a list of distinct days`)

	config.GcpGkeKubernetesRuntimeDefinition = gkeDefinitionValues(util.Ptr("STABLE"), util.Ptr("SU"))
	assert.NoError(t, config.Validate())
}

// TestAGkeDefinitionConfigRecordsTheUpgradeSettingsItsClustersGet covers a
// GKE definition created from config: it records the release channel and
// maintenance window asked for, and the defaults for those left out.
func TestAGkeDefinitionConfigRecordsTheUpgradeSettingsItsClustersGet(t *testing.T) {
	values := gkeDefinitionValues(util.Ptr("STABLE"), util.Ptr("SU"))
	values.MaintenanceWindowStart = util.Ptr("22:00")
	values.MaintenanceWindowHours = util.Ptr(6)
	sent := createGkeDefinition(t, &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: values})
	assert.Equal(t, util.Ptr("STABLE"), sent.ReleaseChannel)
	assert.Equal(t, util.Ptr("SU"), sent.MaintenanceWindowDays)
	assert.Equal(t, util.Ptr("22:00"), sent.MaintenanceWindowStart)
	assert.Equal(t, util.Ptr(6), sent.MaintenanceWindowHours)

	sent = createGkeDefinition(t, &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: gkeDefinitionValues(nil, nil)})
	assert.Equal(t, util.Ptr("REGULAR"), sent.ReleaseChannel)
	assert.Equal(t, util.Ptr("SA,SU"), sent.MaintenanceWindowDays)
	assert.Equal(t, util.Ptr("04:00"), sent.MaintenanceWindowStart)
	assert.Equal(t, util.Ptr(8), sent.MaintenanceWindowHours)
}

// TestAGkeRuntimeConfigSendsItsUpgradeSettings covers a GcpGkeKubernetesRuntime
// config file, which creates the definition and the instance together: the
// definition carries its release channel and maintenance window.
func TestAGkeRuntimeConfigSendsItsUpgradeSettings(t *testing.T) {
	config := GcpGkeKubernetesRuntimeConfig{GcpGkeKubernetesRuntime: GcpGkeKubernetesRuntimeValues{
		Name:                         util.Ptr("gke-remote"),
		ZoneCount:                    util.Ptr(3),
		DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
		DefaultNodeGroupInitialSize:  util.Ptr(1),
		DefaultNodeGroupMinimumSize:  util.Ptr(1),
		DefaultNodeGroupMaximumSize:  util.Ptr(3),
		ReleaseChannel:               util.Ptr("STABLE"),
		MaintenanceWindowDays:        util.Ptr("TU"),
		MaintenanceWindowStart:       util.Ptr("01:00"),
		MaintenanceWindowHours:       util.Ptr(5),
	}}
	api, sent := gkeDefinitionAPI(t)
	operations, definitions, _ := config.GetOperations(api.Client, api.Addr)
	require.NoError(t, operations.Operations[0].Create())
	assert.Equal(t, util.Ptr("STABLE"), (*definitions)[0].GcpGkeKubernetesRuntimeDefinition.ReleaseChannel)

	assert.Equal(t, util.Ptr("STABLE"), sent.ReleaseChannel)
	assert.Equal(t, util.Ptr("TU"), sent.MaintenanceWindowDays)
	assert.Equal(t, util.Ptr("01:00"), sent.MaintenanceWindowStart)
	assert.Equal(t, util.Ptr(5), sent.MaintenanceWindowHours)
}

// TestReplacingAGkeDefinitionKeepsItsUpgradeSettings covers a replace from a
// config that leaves the release channel and maintenance window out: it
// sends the stored ones, which the API refuses to change, rather than
// clearing them.
func TestReplacingAGkeDefinitionKeepsItsUpgradeSettings(t *testing.T) {
	name := "gke-remote"
	stored := api_v0.GcpGkeKubernetesRuntimeDefinition{
		Common:                 api_v0.Common{ID: util.Ptr(uint(1))},
		Definition:             api_v0.Definition{Name: &name},
		ReleaseChannel:         util.Ptr("STABLE"),
		MaintenanceWindowDays:  util.Ptr("SU"),
		MaintenanceWindowStart: util.Ptr("22:00"),
		MaintenanceWindowHours: util.Ptr(6),
	}
	replace := func(values GcpGkeKubernetesRuntimeDefinitionValues) api_v0.GcpGkeKubernetesRuntimeDefinition {
		api := &fakeApi{t: t, objects: map[string]interface{}{api_v0.PathGcpGkeKubernetesRuntimeDefinitions: stored}}
		config := &GcpGkeKubernetesRuntimeDefinitionConfig{GcpGkeKubernetesRuntimeDefinition: values}
		_, err := config.Replace(&http.Client{}, api.serve(), name)
		require.NoError(t, err)
		var sent api_v0.GcpGkeKubernetesRuntimeDefinition
		api.decodeSent(&sent)
		return sent
	}

	sent := replace(gkeDefinitionValues(nil, nil))
	assert.Equal(t, util.Ptr("STABLE"), sent.ReleaseChannel)
	assert.Equal(t, util.Ptr("SU"), sent.MaintenanceWindowDays)
	assert.Equal(t, util.Ptr("22:00"), sent.MaintenanceWindowStart)
	assert.Equal(t, util.Ptr(6), sent.MaintenanceWindowHours)

	values := gkeDefinitionValues(util.Ptr("REGULAR"), util.Ptr("SA"))
	values.MaintenanceWindowStart = util.Ptr("02:00")
	values.MaintenanceWindowHours = util.Ptr(4)
	sent = replace(values)
	assert.Equal(t, util.Ptr("REGULAR"), sent.ReleaseChannel, "a channel the config gives is sent, for the API to refuse")
	assert.Equal(t, util.Ptr("SA"), sent.MaintenanceWindowDays)
	assert.Equal(t, util.Ptr("02:00"), sent.MaintenanceWindowStart)
	assert.Equal(t, util.Ptr(4), sent.MaintenanceWindowHours)
}
