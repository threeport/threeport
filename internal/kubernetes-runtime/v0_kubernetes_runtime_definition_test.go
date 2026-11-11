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

// TestAChangedVersionReachesTheClusters covers a kubernetes runtime
// definition whose KubernetesVersion changed: the EKS or GKE definition
// records it, and each of its instances is marked unreconciled, with a refused
// or failed upgrade cleared, so its controller upgrades the cluster.  An update
// that leaves the version as the provider definition records it reaches
// nothing.
func TestAChangedVersionReachesTheClusters(t *testing.T) {
	for _, c := range []struct {
		provider        string
		definitionsPath string
		instancesPath   string
		instancesQuery  string
	}{
		{v0.KubernetesRuntimeInfraProviderEKS, v0.PathAwsEksKubernetesRuntimeDefinitions, v0.PathAwsEksKubernetesRuntimeInstances, "awsekskubernetesruntimedefinitionid=3"},
		{v0.KubernetesRuntimeInfraProviderGKE, v0.PathGcpGkeKubernetesRuntimeDefinitions, v0.PathGcpGkeKubernetesRuntimeInstances, "gcpgkekubernetesruntimedefinitionid=3"},
	} {
		t.Run(c.provider, func(t *testing.T) {
			run := func(recorded string) (map[string]any, []map[string]any) {
				api := machinetest.NewAPIStub(t)
				var definitionUpdate map[string]any
				var instanceUpdates []map[string]any
				api.Mux.HandleFunc(c.definitionsPath, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "kubernetesruntimedefinitionid=7", r.URL.RawQuery)
					machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{
						map[string]any{"ID": 3, "KubernetesVersion": recorded},
					})
				})
				api.Mux.HandleFunc(c.definitionsPath+"/3", func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, http.MethodPatch, r.Method)
					require.NoError(t, json.NewDecoder(r.Body).Decode(&definitionUpdate))
					machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{definitionUpdate})
				})
				api.Mux.HandleFunc(c.instancesPath, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, c.instancesQuery, r.URL.RawQuery)
					machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{
						map[string]any{"ID": 21, "Name": "one"},
						map[string]any{"ID": 22, "Name": "two"},
					})
				})
				for _, id := range []string{"/21", "/22"} {
					api.Mux.HandleFunc(c.instancesPath+id, func(w http.ResponseWriter, r *http.Request) {
						require.Equal(t, http.MethodPatch, r.Method)
						var update map[string]any
						require.NoError(t, json.NewDecoder(r.Body).Decode(&update))
						instanceUpdates = append(instanceUpdates, update)
						machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{update})
					})
				}

				r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
				log := logr.Discard()
				requeue, err := v0KubernetesRuntimeDefinitionUpdated(r, &v0.KubernetesRuntimeDefinition{
					Common:            v0.Common{ID: util.Ptr(uint(7))},
					InfraProvider:     util.Ptr(c.provider),
					KubernetesVersion: util.Ptr("1.36"),
				}, &log)
				require.NoError(t, err)
				assert.Equal(t, int64(0), requeue)
				return definitionUpdate, instanceUpdates
			}

			definitionUpdate, instanceUpdates := run("1.35")
			assert.Equal(t, "1.36", definitionUpdate["KubernetesVersion"])
			require.Len(t, instanceUpdates, 2)
			for _, update := range instanceUpdates {
				assert.Equal(t, false, update["Reconciled"])
				assert.Equal(t, false, update["UpgradeRefused"])
				assert.Equal(t, false, update["UpgradeFailed"])
			}

			definitionUpdate, instanceUpdates = run("1.36")
			assert.Nil(t, definitionUpdate)
			assert.Empty(t, instanceUpdates)
		})
	}

	// a definition with no version, or on a provider whose clusters aren't
	// upgraded, reaches nothing: the stub serves no route
	api := machinetest.NewAPIStub(t)
	r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr}
	log := logr.Discard()
	for _, definition := range []*v0.KubernetesRuntimeDefinition{
		{Common: v0.Common{ID: util.Ptr(uint(7))}, InfraProvider: util.Ptr(v0.KubernetesRuntimeInfraProviderEKS)},
		{Common: v0.Common{ID: util.Ptr(uint(7))}, InfraProvider: util.Ptr(v0.KubernetesRuntimeInfraProviderOKE), KubernetesVersion: util.Ptr("1.36")},
		{Common: v0.Common{ID: util.Ptr(uint(7))}, InfraProvider: util.Ptr(v0.KubernetesRuntimeInfraProviderKind)},
	} {
		_, err := v0KubernetesRuntimeDefinitionUpdated(r, definition, &log)
		require.NoError(t, err)
	}
}
