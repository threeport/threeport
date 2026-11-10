package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAProductionControlPlanesGkeClusterIsOnTheStableChannel covers the
// release channel of the GKE cluster tptctl up creates for each tier.
func TestAProductionControlPlanesGkeClusterIsOnTheStableChannel(t *testing.T) {
	assert.Equal(t, "STABLE", gkeReleaseChannel(threeport.ControlPlaneTier(threeport.ControlPlaneTierProd)))
	assert.Equal(t, "REGULAR", gkeReleaseChannel(threeport.ControlPlaneTier(threeport.ControlPlaneTierDev)))
	assert.Equal(t, "REGULAR", gkeReleaseChannel(""))
}

// TestTheGenesisGkeDefinitionRecordsTheClustersUpgradeSettings covers the
// release channel and maintenance window recorded on the GKE definition of
// the cluster tptctl up creates: the ones it was created with, and the
// defaults for the ones it left unset.
func TestTheGenesisGkeDefinitionRecordsTheClustersUpgradeSettings(t *testing.T) {
	var definition v0.GcpGkeKubernetesRuntimeDefinition
	recordGkeUpgradeSettings(&definition, &provider.KubernetesRuntimeInfraGKE{ReleaseChannel: "STABLE"})
	assert.Equal(t, "STABLE", *definition.ReleaseChannel)
	assert.Equal(t, "SA,SU", *definition.MaintenanceWindowDays)
	assert.Equal(t, "04:00", *definition.MaintenanceWindowStart)
	assert.Equal(t, 8, *definition.MaintenanceWindowHours)

	definition = v0.GcpGkeKubernetesRuntimeDefinition{}
	recordGkeUpgradeSettings(&definition, &provider.KubernetesRuntimeInfraGKE{
		MaintenanceWindowDays:  "WE",
		MaintenanceWindowStart: "01:30",
		MaintenanceWindowHours: 5,
	})
	assert.Equal(t, "REGULAR", *definition.ReleaseChannel)
	assert.Equal(t, "WE", *definition.MaintenanceWindowDays)
	assert.Equal(t, "01:30", *definition.MaintenanceWindowStart)
	assert.Equal(t, 5, *definition.MaintenanceWindowHours)
}

// TestAGkeClusterTptctlDidntCreateRecordsNoUpgradeSettings covers the GKE
// definition registered with --control-plane-only: its cluster's channel and
// window are whatever its owner set, so none is recorded.
func TestAGkeClusterTptctlDidntCreateRecordsNoUpgradeSettings(t *testing.T) {
	var sent v0.GcpGkeKubernetesRuntimeDefinition
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"Data":[]}`)
			return
		}
		if r.URL.Path == v0.PathGcpGkeKubernetesRuntimeDefinitions {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"Data":[{"ID":4}]}`)
	}))
	defer server.Close()

	cpi := &threeport.ControlPlaneInstaller{Opts: threeport.Options{
		ControlPlaneName: "test",
		ControlPlaneOnly: true,
		Tier:             threeport.ControlPlaneTier(threeport.ControlPlaneTierProd),
	}}
	var infra provider.KubernetesRuntimeInfra = &provider.KubernetesRuntimeInfraGKE{
		ProjectID:      "project",
		Region:         "us-central1",
		ReleaseChannel: "STABLE",
	}
	require.NoError(t, ConfigureControlPlaneWithGkeConfig(
		cpi,
		&Uninstaller{},
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.KubernetesRuntimeDefinition{Common: v0.Common{ID: util.Ptr(uint(2))}},
		&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}},
		&infra,
	))
	require.NotNil(t, sent.ZoneCount, "the definition was sent")
	assert.Nil(t, sent.ReleaseChannel)
	assert.Nil(t, sent.MaintenanceWindowDays)
	assert.Nil(t, sent.MaintenanceWindowStart)
	assert.Nil(t, sent.MaintenanceWindowHours)
}
