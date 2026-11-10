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

// TestTheKubernetesVersionFlagNamesAVersionTptctlCreatesAClusterOn covers
// --kubernetes-version: a supported version on a provider whose cluster
// tptctl creates.
func TestTheKubernetesVersionFlagNamesAVersionTptctlCreatesAClusterOn(t *testing.T) {
	eks := v0.KubernetesRuntimeInfraProviderEKS
	assert.NoError(t, ValidateKubernetesVersionFlag("", eks, false))
	assert.NoError(t, ValidateKubernetesVersionFlag("", eks, true))
	assert.NoError(t, ValidateKubernetesVersionFlag("1.35", eks, false))

	assert.ErrorContains(t, ValidateKubernetesVersionFlag("1.32", eks, false), `--kubernetes-version: KubernetesVersion "1.32" is not supported`)
	assert.ErrorContains(t, ValidateKubernetesVersionFlag("1.35", v0.KubernetesRuntimeInfraProviderKind, false), "cannot be set on a kind cluster")
	assert.ErrorContains(t, ValidateKubernetesVersionFlag("1.35", eks, true), "does not apply with --control-plane-only")
}

// TestTheGenesisDefinitionRecordsTheVersionTptctlCreatesItsClusterOn covers
// the version on the definition of the cluster tptctl up creates.
func TestTheGenesisDefinitionRecordsTheVersionTptctlCreatesItsClusterOn(t *testing.T) {
	assert.Equal(t, "1.35", *genesisKubernetesVersion(threeport.Options{
		InfraProvider: v0.KubernetesRuntimeInfraProviderGKE, KubernetesVersion: "1.35",
	}))
	assert.Equal(t, v0.DefaultKubernetesVersion, *genesisKubernetesVersion(threeport.Options{
		InfraProvider: v0.KubernetesRuntimeInfraProviderOKE,
	}))
	assert.Nil(t, genesisKubernetesVersion(threeport.Options{InfraProvider: v0.KubernetesRuntimeInfraProviderKind}))
}

// TestTheGenesisGkeDefinitionRecordsTheClustersVersion covers the GKE
// definition tptctl up registers for the cluster it runs on: it carries the
// version of that cluster's kubernetes runtime definition, so a runtime
// created from it later gets the same version, and none for a cluster tptctl
// didn't create.
func TestTheGenesisGkeDefinitionRecordsTheClustersVersion(t *testing.T) {
	for name, version := range map[string]*string{"created by tptctl": util.Ptr("1.35"), "created elsewhere": nil} {
		t.Run(name, func(t *testing.T) {
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

			cpi := &threeport.ControlPlaneInstaller{Opts: threeport.Options{ControlPlaneName: "test", ControlPlaneOnly: true}}
			var infra provider.KubernetesRuntimeInfra = &provider.KubernetesRuntimeInfraGKE{ProjectID: "project", Region: "us-central1"}
			require.NoError(t, ConfigureControlPlaneWithGkeConfig(
				cpi,
				&Uninstaller{},
				server.Client(),
				strings.TrimPrefix(server.URL, "http://"),
				&v0.KubernetesRuntimeDefinition{Common: v0.Common{ID: util.Ptr(uint(2))}, KubernetesVersion: version},
				&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}},
				&infra,
			))
			assert.Equal(t, version, sent.KubernetesVersion)
		})
	}
}
