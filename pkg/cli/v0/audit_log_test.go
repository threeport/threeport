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

// TestAProductionControlPlanesClusterHasAnAuditLog covers the cluster tptctl
// up creates: a production control plane's has its Kubernetes API audit log
// enabled, a development one's doesn't, and a kind cluster has nowhere to
// send one.
func TestAProductionControlPlanesClusterHasAnAuditLog(t *testing.T) {
	production := threeport.ControlPlaneTier(threeport.ControlPlaneTierProd)
	development := threeport.ControlPlaneTier(threeport.DefaultControlPlaneTier)
	for _, infraProvider := range []string{
		v0.KubernetesRuntimeInfraProviderEKS,
		v0.KubernetesRuntimeInfraProviderGKE,
		v0.KubernetesRuntimeInfraProviderOKE,
	} {
		assert.True(t, genesisAuditLog(threeport.Options{InfraProvider: infraProvider, Tier: production}), infraProvider)
		assert.False(t, genesisAuditLog(threeport.Options{InfraProvider: infraProvider, Tier: development}), infraProvider)
	}
	assert.False(t, genesisAuditLog(threeport.Options{InfraProvider: v0.KubernetesRuntimeInfraProviderKind, Tier: production}))
}

// TestTheGenesisGkeDefinitionRecordsItsAuditLog covers the GKE definition
// tptctl up registers for the cluster it runs on: it records the audit log of
// the cluster's kubernetes runtime definition.
func TestTheGenesisGkeDefinitionRecordsItsAuditLog(t *testing.T) {
	for _, auditLog := range []*bool{util.Ptr(true), util.Ptr(false)} {
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

		cpi := &threeport.ControlPlaneInstaller{Opts: threeport.Options{ControlPlaneName: "test", ControlPlaneOnly: true}}
		var infra provider.KubernetesRuntimeInfra = &provider.KubernetesRuntimeInfraGKE{ProjectID: "project", Region: "us-central1"}
		require.NoError(t, ConfigureControlPlaneWithGkeConfig(
			cpi,
			&Uninstaller{},
			server.Client(),
			strings.TrimPrefix(server.URL, "http://"),
			&v0.KubernetesRuntimeDefinition{Common: v0.Common{ID: util.Ptr(uint(2))}, AuditLog: auditLog},
			&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}},
			&infra,
		))
		server.Close()
		assert.Equal(t, auditLog, sent.AuditLog)
	}
}
