package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAProductionControlPlanesClusterIsProtected covers the cluster tptctl up
// creates: a production control plane's is protected from deletion, a
// development one's isn't, and a kind cluster has nothing to protect.
func TestAProductionControlPlanesClusterIsProtected(t *testing.T) {
	production := threeport.ControlPlaneTier(threeport.ControlPlaneTierProd)
	development := threeport.ControlPlaneTier(threeport.DefaultControlPlaneTier)
	for _, infraProvider := range []string{
		v0.KubernetesRuntimeInfraProviderEKS,
		v0.KubernetesRuntimeInfraProviderGKE,
		v0.KubernetesRuntimeInfraProviderOKE,
	} {
		assert.True(t, genesisDeletionProtection(threeport.Options{InfraProvider: infraProvider, Tier: production}), infraProvider)
		assert.False(t, genesisDeletionProtection(threeport.Options{InfraProvider: infraProvider, Tier: development}), infraProvider)
	}
	assert.False(t, genesisDeletionProtection(threeport.Options{InfraProvider: v0.KubernetesRuntimeInfraProviderKind, Tier: production}))
}

// TestTheGenesisGkeDefinitionRecordsItsDeletionProtection covers the GKE
// definition tptctl up registers for the cluster it runs on: it is protected
// as the cluster's kubernetes runtime definition is, since its
// DeletionProtection is the one a delete is checked against.
func TestTheGenesisGkeDefinitionRecordsItsDeletionProtection(t *testing.T) {
	for _, protection := range []*bool{util.Ptr(true), util.Ptr(false)} {
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
			&v0.KubernetesRuntimeDefinition{Common: v0.Common{ID: util.Ptr(uint(2))}, DeletionProtection: protection},
			&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}},
			&infra,
		))
		server.Close()
		assert.Equal(t, protection, sent.DeletionProtection)
	}
}

// teardownAPI serves the control plane's runtime instance, its EKS instance
// and its EKS definition, with DeletionProtection as given.  status, when
// set, is what every call answers instead; delay holds every answer.
func teardownAPI(t *testing.T, protection *bool, status int, delay time.Duration) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"Status":{"Error":"unauthorized"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasPrefix(r.URL.Path, v0.PathAwsEksKubernetesRuntimeDefinitions):
			body, err := json.Marshal(map[string]any{"Data": []any{map[string]any{
				"ID": 4, "Name": "threeport-prod", "DeletionProtection": protection,
			}}})
			require.NoError(t, err)
			_, _ = w.Write(body)
		case strings.HasPrefix(r.URL.Path, v0.PathAwsEksKubernetesRuntimeInstances):
			fmt.Fprint(w, `{"Data":[{"ID":9,"AwsEksKubernetesRuntimeDefinitionID":4}]}`)
		default:
			fmt.Fprint(w, `{"Data":[{"ID":3,"ThreeportControlPlaneHost":true}]}`)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

// TestTptctlDownRefusesToTearDownAProtectedCluster covers the check tptctl
// down makes before deleting anything: it refuses while the cluster's EKS,
// GKE or OKE definition is protected, saying how to turn the protection off,
// and goes ahead once it is off.  A kind cluster isn't checked.
func TestTptctlDownRefusesToTearDownAProtectedCluster(t *testing.T) {
	instance := &v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}}
	for _, infraOnly := range []bool{false, true} {
		server := teardownAPI(t, util.Ptr(true), 0, 0)
		endpoint := strings.TrimPrefix(server.URL, "http://")
		given := instance
		if infraOnly {
			given = nil
		}

		skipped, err := checkTeardownDeletionProtection(server.Client(), endpoint, v0.KubernetesRuntimeInfraProviderEKS, given, infraOnly)
		assert.Nil(t, skipped)
		var protected *v0.DeletionProtectedError
		require.True(t, errors.As(err, &protected), "infra only %t: %v", infraOnly, err)
		assert.Contains(t, err.Error(), "tptctl replace aws-eks-kubernetes-runtime-definition --config <file> --name threeport-prod")
		assert.Contains(t, err.Error(), "then run tptctl down again")

		for _, protection := range []*bool{util.Ptr(false), nil} {
			server = teardownAPI(t, protection, 0, 0)
			skipped, err = checkTeardownDeletionProtection(
				server.Client(), strings.TrimPrefix(server.URL, "http://"), v0.KubernetesRuntimeInfraProviderEKS, given, infraOnly,
			)
			assert.NoError(t, err)
			assert.Nil(t, skipped)
		}
	}

	skipped, err := checkTeardownDeletionProtection(&http.Client{}, "127.0.0.1:1", v0.KubernetesRuntimeInfraProviderKind, nil, true)
	assert.NoError(t, err)
	assert.Nil(t, skipped)
}

// TestTptctlDownInfraOnlySkipsTheCheckOnlyWithoutAnApi covers --infra-only,
// which tears down a cluster whose control plane may be gone: when the API
// can't be reached, or doesn't answer in time, the check is skipped and the
// teardown goes ahead, while an API that answers with an error is running,
// so the teardown is refused.
func TestTptctlDownInfraOnlySkipsTheCheckOnlyWithoutAnApi(t *testing.T) {
	timeout := teardownProtectionCheckTimeout
	teardownProtectionCheckTimeout = 50 * time.Millisecond
	t.Cleanup(func() { teardownProtectionCheckTimeout = timeout })

	// nothing listening
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedEndpoint := listener.Addr().String()
	require.NoError(t, listener.Close())
	skipped, err := checkTeardownDeletionProtection(&http.Client{}, closedEndpoint, v0.KubernetesRuntimeInfraProviderEKS, nil, true)
	assert.NoError(t, err)
	assert.Error(t, skipped, "a refused connection skips the check")

	// no answer in time
	slow := teardownAPI(t, util.Ptr(true), 0, time.Second)
	skipped, err = checkTeardownDeletionProtection(slow.Client(), strings.TrimPrefix(slow.URL, "http://"), v0.KubernetesRuntimeInfraProviderEKS, nil, true)
	assert.NoError(t, err)
	assert.Error(t, skipped, "an API that doesn't answer in time skips the check")

	// an answer that is an error
	failing := teardownAPI(t, util.Ptr(false), http.StatusUnauthorized, 0)
	skipped, err = checkTeardownDeletionProtection(failing.Client(), strings.TrimPrefix(failing.URL, "http://"), v0.KubernetesRuntimeInfraProviderEKS, nil, true)
	assert.Nil(t, skipped)
	assert.ErrorContains(t, err, "the Threeport API answered with an error")
	assert.ErrorContains(t, err, "unauthorized")

	// without --infra-only the API was already reached, so a failure isn't skipped
	skipped, err = checkTeardownDeletionProtection(
		&http.Client{}, closedEndpoint, v0.KubernetesRuntimeInfraProviderEKS,
		&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}}, false,
	)
	assert.Nil(t, skipped)
	assert.Error(t, err)
}

// TestTptctlDownControlPlaneOnlyLeavesAProtectedClusterAlone covers
// --control-plane-only, which removes the control plane and leaves its
// cluster as it is: a protected cluster doesn't stop it, while a teardown of
// the cluster is refused.
func TestTptctlDownControlPlaneOnlyLeavesAProtectedClusterAlone(t *testing.T) {
	server := teardownAPI(t, util.Ptr(true), 0, 0)
	controlPlane := &ControlPlane{
		APIServer: strings.TrimPrefix(server.URL, "http://"),
		Provider:  v0.KubernetesRuntimeInfraProviderEKS,
	}
	instance := &v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(3))}}

	assert.NoError(t, checkTeardownProtection(server.Client(), controlPlane, instance, threeport.Options{ControlPlaneOnly: true}))
	var protected *v0.DeletionProtectedError
	assert.True(t, errors.As(checkTeardownProtection(server.Client(), controlPlane, instance, threeport.Options{}), &protected))
}
