package kubernetesruntime

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	logr "github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestTheProviderDefinitionCarriesTheKubernetesApiAccess covers #729: the
// EKS or GKE definition the reconciler creates from a kubernetes runtime
// definition carries its PrivateApi and AllowedCidrs, which is where the
// provider's reconcilers will read them.
func TestTheProviderDefinitionCarriesTheKubernetesApiAccess(t *testing.T) {
	allowed := datatypes.JSON(`["203.0.113.0/24","10.8.0.0/16"]`)

	cases := []struct {
		provider     string
		path         string
		privateApi   *bool
		allowedCidrs *datatypes.JSON
		wantPrivate  *bool
		wantCidrs    []string
	}{
		{v0.KubernetesRuntimeInfraProviderEKS, v0.PathAwsEksKubernetesRuntimeDefinitions, util.Ptr(true), &allowed, util.Ptr(true), []string{"203.0.113.0/24", "10.8.0.0/16"}},
		{v0.KubernetesRuntimeInfraProviderGKE, v0.PathGcpGkeKubernetesRuntimeDefinitions, util.Ptr(true), &allowed, util.Ptr(true), []string{"203.0.113.0/24", "10.8.0.0/16"}},
		{v0.KubernetesRuntimeInfraProviderEKS, v0.PathAwsEksKubernetesRuntimeDefinitions, util.Ptr(false), nil, util.Ptr(false), nil},
		{v0.KubernetesRuntimeInfraProviderGKE, v0.PathGcpGkeKubernetesRuntimeDefinitions, util.Ptr(false), nil, util.Ptr(false), nil},
	}
	for _, c := range cases {
		name := c.provider + " public"
		if *c.privateApi {
			name = c.provider + " private with an allowlist"
		}
		t.Run(name, func(t *testing.T) {
			api := machinetest.NewAPIStub(t)
			var sent map[string]any
			api.Mux.HandleFunc(c.path, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(body, &sent))
				machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent})
			})
			r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
			log := logr.Discard()

			definition := &v0.KubernetesRuntimeDefinition{
				Common:           v0.Common{ID: util.Ptr(uint(7))},
				Definition:       v0.Definition{Name: util.Ptr("remote")},
				Reconciliation:   v0.Reconciliation{Reconciled: util.Ptr(false)},
				InfraProvider:    util.Ptr(c.provider),
				HighAvailability: util.Ptr(false),
				NodeSize:         util.Ptr("Medium"),
				NodeProfile:      util.Ptr("Balanced"),
				NodeMaximum:      util.Ptr(10),
				PrivateApi:       c.privateApi,
				AllowedCidrs:     c.allowedCidrs,
			}
			_, err := v0KubernetesRuntimeDefinitionCreated(r, definition, &log)
			require.NoError(t, err)
			require.NotNil(t, sent, "the reconciler creates the provider definition")

			assert.Equal(t, *c.wantPrivate, sent["PrivateApi"])
			if c.wantCidrs == nil {
				assert.Nil(t, sent["AllowedCidrs"])
				return
			}
			var cidrs []string
			raw, err := json.Marshal(sent["AllowedCidrs"])
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &cidrs))
			assert.Equal(t, c.wantCidrs, cidrs)
		})
	}
}

// TestTheProviderDefinitionRecordsTheKubernetesVersion covers the version on
// the EKS or GKE definition the reconciler creates: the one the kubernetes
// runtime definition asks for, or the default, so the provider definition
// records the version its clusters are created on.
func TestTheProviderDefinitionRecordsTheKubernetesVersion(t *testing.T) {
	for _, c := range []struct {
		provider string
		path     string
		asked    *string
		want     string
	}{
		{v0.KubernetesRuntimeInfraProviderEKS, v0.PathAwsEksKubernetesRuntimeDefinitions, util.Ptr("1.35"), "1.35"},
		{v0.KubernetesRuntimeInfraProviderGKE, v0.PathGcpGkeKubernetesRuntimeDefinitions, util.Ptr("1.35"), "1.35"},
		{v0.KubernetesRuntimeInfraProviderEKS, v0.PathAwsEksKubernetesRuntimeDefinitions, nil, v0.DefaultKubernetesVersion},
		{v0.KubernetesRuntimeInfraProviderGKE, v0.PathGcpGkeKubernetesRuntimeDefinitions, nil, v0.DefaultKubernetesVersion},
	} {
		t.Run(c.provider+" "+c.want, func(t *testing.T) {
			api := machinetest.NewAPIStub(t)
			var sent map[string]any
			api.Mux.HandleFunc(c.path, func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(body, &sent))
				machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent})
			})
			r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
			log := logr.Discard()

			definition := &v0.KubernetesRuntimeDefinition{
				Common:            v0.Common{ID: util.Ptr(uint(7))},
				Definition:        v0.Definition{Name: util.Ptr("remote")},
				Reconciliation:    v0.Reconciliation{Reconciled: util.Ptr(false)},
				InfraProvider:     util.Ptr(c.provider),
				HighAvailability:  util.Ptr(false),
				NodeSize:          util.Ptr("Medium"),
				NodeProfile:       util.Ptr("Balanced"),
				NodeMaximum:       util.Ptr(10),
				KubernetesVersion: c.asked,
			}
			_, err := v0KubernetesRuntimeDefinitionCreated(r, definition, &log)
			require.NoError(t, err)
			assert.Equal(t, c.want, sent["KubernetesVersion"])
		})
	}
}

// TestTheGkeDefinitionRecordsItsUpgradeSettings covers the release channel
// and maintenance window on the GKE definition the reconciler creates: the
// defaults, recorded so the definition shows when its clusters are upgraded.
func TestTheGkeDefinitionRecordsItsUpgradeSettings(t *testing.T) {
	api := machinetest.NewAPIStub(t)
	var sent map[string]any
	api.Mux.HandleFunc(v0.PathGcpGkeKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &sent))
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent})
	})
	r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
	log := logr.Discard()

	definition := &v0.KubernetesRuntimeDefinition{
		Common:           v0.Common{ID: util.Ptr(uint(7))},
		Definition:       v0.Definition{Name: util.Ptr("remote")},
		Reconciliation:   v0.Reconciliation{Reconciled: util.Ptr(false)},
		InfraProvider:    util.Ptr(v0.KubernetesRuntimeInfraProviderGKE),
		HighAvailability: util.Ptr(false),
		NodeSize:         util.Ptr("Medium"),
		NodeProfile:      util.Ptr("Balanced"),
		NodeMaximum:      util.Ptr(10),
	}
	_, err := v0KubernetesRuntimeDefinitionCreated(r, definition, &log)
	require.NoError(t, err)
	assert.Equal(t, "REGULAR", sent["ReleaseChannel"])
	assert.Equal(t, "SA,SU", sent["MaintenanceWindowDays"])
	assert.Equal(t, "04:00", sent["MaintenanceWindowStart"])
	assert.Equal(t, float64(8), sent["MaintenanceWindowHours"])
}

// TestANewGkeWorkloadClusterReservesItsNatAddress covers the GKE instance
// the reconciler creates for a new kubernetes runtime instance: it is a new
// cluster, so its Cloud NAT gets a reserved address that the cluster's
// EgressCidrs record.
func TestANewGkeWorkloadClusterReservesItsNatAddress(t *testing.T) {
	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(v0.PathKubernetesRuntimeDefinitions+"/3", func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{v0.KubernetesRuntimeDefinition{
			Common:                   v0.Common{ID: util.Ptr(uint(3))},
			InfraProvider:            util.Ptr(v0.KubernetesRuntimeInfraProviderGKE),
			InfraProviderAccountName: util.Ptr("gcp-account"),
		}})
	})
	api.Mux.HandleFunc(v0.PathGcpGkeKubernetesRuntimeDefinitions, func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{v0.GcpGkeKubernetesRuntimeDefinition{
			Common: v0.Common{ID: util.Ptr(uint(2))},
		}})
	})
	api.Mux.HandleFunc(v0.PathGcpProviders, func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{v0.GcpProvider{
			Common: v0.Common{ID: util.Ptr(uint(1))}, Name: util.Ptr("gcp-account"),
		}})
	})
	var sent v0.GcpGkeKubernetesRuntimeInstance
	api.Mux.HandleFunc(v0.PathGcpGkeKubernetesRuntimeInstances, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		machinetest.WriteResponse(t, w, http.StatusCreated, []apiserver_lib.Object{sent})
	})
	r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
	log := logr.Discard()

	_, err := v0KubernetesRuntimeInstanceCreated(r, &v0.KubernetesRuntimeInstance{
		Common:                        v0.Common{ID: util.Ptr(uint(4))},
		Instance:                      v0.Instance{Name: util.Ptr("gke-remote")},
		Reconciliation:                v0.Reconciliation{Reconciled: util.Ptr(false)},
		Location:                      util.Ptr("NorthAmerica:NewYork"),
		KubernetesRuntimeDefinitionID: util.Ptr(uint(3)),
	}, &log)
	require.NoError(t, err)
	assert.Equal(t, util.Ptr(true), sent.ReservedNatAddress)
}
