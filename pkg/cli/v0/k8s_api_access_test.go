package v0

import (
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	provider "github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestTheK8sApiAccessFlagsAreCheckedBeforeAnythingIsCreated covers
// --private-k8s-api and --k8s-api-allowed-cidrs: they apply to a cluster
// tptctl creates on a cloud provider, a private API only to GKE for now, and
// the blocks are checked against the provider's limit, which is higher for a
// private GKE API.
func TestTheK8sApiAccessFlagsAreCheckedBeforeAnythingIsCreated(t *testing.T) {
	blocks := func(n int) []string {
		cidrs := make([]string, n)
		for i := range cidrs {
			cidrs[i] = fmt.Sprintf("198.51.%d.%d/32", i/256, i%256)
		}
		return cidrs
	}

	cases := []struct {
		name             string
		private          bool
		cidrs            []string
		provider         string
		controlPlaneOnly bool
		wantErr          string
	}{
		{"not set", false, nil, v0.KubernetesRuntimeInfraProviderEKS, false, ""},
		{"not set on kind", false, nil, v0.KubernetesRuntimeInfraProviderKind, false, ""},
		{"not set with control plane only", false, nil, v0.KubernetesRuntimeInfraProviderGKE, true, ""},
		{"set on EKS", false, []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderEKS, false, ""},
		{"set on GKE", false, []string{"203.0.113.0/24", "10.8.0.0/16"}, v0.KubernetesRuntimeInfraProviderGKE, false, ""},
		{"set on OKE", false, []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderOKE, false, ""},
		{"on kind", false, []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderKind, false, "--k8s-api-allowed-cidrs is not supported with infrastructure provider 'kind'"},
		{"with control plane only", false, []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderEKS, true, "--k8s-api-allowed-cidrs is not supported with --control-plane-only"},
		{"an address without a prefix", false, []string{"203.0.113.7"}, v0.KubernetesRuntimeInfraProviderEKS, false, `--k8s-api-allowed-cidrs entry "203.0.113.7" is not a CIDR block`},
		{"host bits set", false, []string{"203.0.113.7/24"}, v0.KubernetesRuntimeInfraProviderGKE, false, `write it as "203.0.113.0/24"`},
		{"EKS at its limit", false, blocks(40), v0.KubernetesRuntimeInfraProviderEKS, false, ""},
		{"EKS past its limit", false, blocks(41), v0.KubernetesRuntimeInfraProviderEKS, false, "has 41 entries and eks admits at most 40"},
		{"GKE past its public limit", false, blocks(51), v0.KubernetesRuntimeInfraProviderGKE, false, "admits at most 50"},
		{"private on GKE", true, nil, v0.KubernetesRuntimeInfraProviderGKE, false, ""},
		{"private on GKE with private networks", true, []string{"10.8.0.0/16"}, v0.KubernetesRuntimeInfraProviderGKE, false, ""},
		{"private GKE at its limit", true, blocks(100), v0.KubernetesRuntimeInfraProviderGKE, false, ""},
		{"private GKE past its limit", true, blocks(101), v0.KubernetesRuntimeInfraProviderGKE, false, "has 101 entries and gke admits at most 100"},
		{"private on EKS", true, []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderEKS, false, `--private-k8s-api is supported only with infrastructure provider 'gke' for now, not "eks"`},
		{"private on OKE", true, nil, v0.KubernetesRuntimeInfraProviderOKE, false, `not "oke"`},
		{"private on kind", true, nil, v0.KubernetesRuntimeInfraProviderKind, false, "--private-k8s-api is not supported with infrastructure provider 'kind'"},
		{"private with control plane only", true, nil, v0.KubernetesRuntimeInfraProviderGKE, true, "--private-k8s-api is not supported with --control-plane-only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateK8sApiAccessFlags(c.private, c.cidrs, c.provider, c.controlPlaneOnly)
			if c.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
		})
	}
}

// TestAllowedCidrsAreSentOnlyWhenSet covers the JSON the control plane's
// definitions are given: nothing without the flag, so they read as they did.
func TestAllowedCidrsAreSentOnlyWhenSet(t *testing.T) {
	assert.Nil(t, allowedCidrsJSON(nil))
	assert.Nil(t, allowedCidrsJSON([]string{}))
	got := allowedCidrsJSON([]string{"203.0.113.7/32"})
	require.NotNil(t, got)
	assert.JSONEq(t, `["203.0.113.7/32"]`, string(*got))
}

// shortenKubeAPIReach makes waitForKubernetesAPI give up within a second.
func shortenKubeAPIReach(t *testing.T) {
	t.Helper()
	timeout, attempt, wait := kubeAPIReachTimeout, kubeAPIReachAttemptTimeout, kubeAPIReachRetryWait
	kubeAPIReachTimeout, kubeAPIReachAttemptTimeout, kubeAPIReachRetryWait = time.Second, 200*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() {
		kubeAPIReachTimeout, kubeAPIReachAttemptTimeout, kubeAPIReachRetryWait = timeout, attempt, wait
	})
}

// runtimeInstanceFor returns a runtime instance that reaches endpoint with
// a bearer token, trusting caPEM.
func runtimeInstanceFor(endpoint, caPEM string) *v0.KubernetesRuntimeInstance {
	return &v0.KubernetesRuntimeInstance{
		APIEndpoint:     util.Ptr(endpoint),
		CACertificate:   util.Ptr(caPEM),
		ConnectionToken: util.Ptr("token"),
	}
}

// kubeAPIServer serves /version with status, and counts the requests.
func kubeAPIServer(t *testing.T, status int) (*v0.KubernetesRuntimeInstance, *int64) {
	t.Helper()
	var requests int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		assert.Equal(t, "/version", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"major":"1","minor":"33","gitVersion":"v1.33.0"}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","code":%d}`, status)
	}))
	t.Cleanup(srv.Close)
	caPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	return runtimeInstanceFor(srv.URL, caPEM), &requests
}

// TestTptctlUpChecksItCanReachTheKubernetesApi covers the check tptctl up
// makes before installing on a cluster whose API is limited to
// --k8s-api-allowed-cidrs.
func TestTptctlUpChecksItCanReachTheKubernetesApi(t *testing.T) {
	t.Run("an API that answers", func(t *testing.T) {
		shortenKubeAPIReach(t)
		instance, requests := kubeAPIServer(t, http.StatusOK)
		require.NoError(t, waitForKubernetesAPI(instance, []string{"203.0.113.7/32"}))
		assert.Equal(t, int64(1), atomic.LoadInt64(requests))
	})
	t.Run("an API that refuses the request is still reachable", func(t *testing.T) {
		shortenKubeAPIReach(t)
		instance, _ := kubeAPIServer(t, http.StatusUnauthorized)
		require.NoError(t, waitForKubernetesAPI(instance, []string{"203.0.113.7/32"}))
	})
	t.Run("an API that cannot be reached", func(t *testing.T) {
		shortenKubeAPIReach(t)
		// a port nothing listens on, which is how a refused address looks
		// from the outside on a provider that closes the connection
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		endpoint := "https://" + listener.Addr().String()
		require.NoError(t, listener.Close())

		started := time.Now()
		err = waitForKubernetesAPI(runtimeInstanceFor(endpoint, ""), []string{"203.0.113.7/32"})
		require.Error(t, err)
		assert.Less(t, time.Since(started), 3*time.Second, "it gives up at its deadline")
		assert.Contains(t, err.Error(), "could not reach the Kubernetes API at "+endpoint)
		assert.Contains(t, err.Error(), "this machine's public address is not in --k8s-api-allowed-cidrs [203.0.113.7/32]")
	})
	t.Run("an API that never answers", func(t *testing.T) {
		shortenKubeAPIReach(t)
		// a provider that drops the packets leaves the connection hanging
		hang := make(chan struct{})
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-hang
		}))
		t.Cleanup(func() {
			close(hang)
			srv.Close()
		})
		caPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))

		started := time.Now()
		err := waitForKubernetesAPI(runtimeInstanceFor(srv.URL, caPEM), []string{"203.0.113.7/32"})
		require.Error(t, err)
		assert.Less(t, time.Since(started), 3*time.Second, "each attempt is cut off rather than left to hang")
		assert.Contains(t, err.Error(), "--k8s-api-allowed-cidrs")
	})
}

// TestAnUnreachableClusterComesWithACommandThatDeletesIt covers the advice
// given when the check fails: a command that runs as printed, naming the
// control plane as tptctl down requires, and none when the cluster is torn
// down anyway.
func TestAnUnreachableClusterComesWithACommandThatDeletesIt(t *testing.T) {
	cause := errors.New("could not reach the Kubernetes API")

	kept := withUnreachableClusterCleanup(cause, "prod-east", false)
	assert.ErrorIs(t, kept, cause)
	assert.Contains(t, kept.Error(), "'tptctl down --name prod-east --infra-only'")
	assert.Contains(t, kept.Error(), "--teardown-on-failure")

	assert.Equal(t, cause, withUnreachableClusterCleanup(cause, "prod-east", true))
}

// TestAReinstallRecordsTheControlPlanesEgressCidrsAgain covers a reinstall,
// which rebuilds the runtime hosting the control plane from the tptctl
// config: the addresses tptctl up recorded there are restored with it, so
// the clusters the control plane manages keep admitting it.
func TestAReinstallRecordsTheControlPlanesEgressCidrsAgain(t *testing.T) {
	config := &ControlPlane{
		Name:              "prod-east",
		Provider:          v0.KubernetesRuntimeInfraProviderGKE,
		GKEProviderConfig: GKEProviderConfig{GcpRegion: "us-central1"},
		KubeAPI:           KubeAPI{APIEndpoint: "https://gke.example.com", Token: util.Base64Encode("token")},
		EgressCidrs:       []string{"198.51.100.20/32", "198.51.100.21/32"},
	}
	instance, err := bootstrapKubernetesRuntimeInstance(config)
	require.NoError(t, err)
	require.NotNil(t, instance.EgressCidrs)
	assert.JSONEq(t, `["198.51.100.20/32","198.51.100.21/32"]`, string(*instance.EgressCidrs))

	config.EgressCidrs = nil
	instance, err = bootstrapKubernetesRuntimeInstance(config)
	require.NoError(t, err)
	assert.Nil(t, instance.EgressCidrs, "a control plane installed before has none recorded")
}

// useTestThreeportConfig points the tptctl config at an empty file for the
// test, and returns the path of a kubeconfig for a cluster named
// threeport-split, which tptctl reads under --control-plane-only on kind.
func useTestThreeportConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
current-context: threeport-split
clusters:
- name: threeport-split
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: threeport-split
  context:
    cluster: threeport-split
    user: threeport-split
users:
- name: threeport-split
  user:
    token: token
`), 0600))

	configFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, nil, 0600))
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigFile(configFile)
	return kubeconfig
}

// TestASplitInstallKeepsWhatTheInfraOnlyRunRecorded covers --control-plane-only
// after --infra-only: the second run reads neither list from the cluster or
// a flag, so it keeps the ones the first run recorded.
func TestASplitInstallKeepsWhatTheInfraOnlyRunRecorded(t *testing.T) {
	kubeconfig := useTestThreeportConfig(t)
	_, err := (&ControlPlane{
		Name:               "split",
		Provider:           v0.KubernetesRuntimeInfraProviderKind,
		K8sApiAllowedCidrs: []string{"203.0.113.7/32"},
		PrivateK8sApi:      true,
		EgressCidrs:        []string{"198.51.100.20/32"},
	}).UpdateThreeportConfigInstance(func(*ControlPlane) {})
	require.NoError(t, err)

	cpi := threeport.NewInstaller()
	cpi.Opts.ControlPlaneName = "split"
	cpi.Opts.InfraProvider = v0.KubernetesRuntimeInfraProviderKind
	cpi.Opts.KubeconfigPath = kubeconfig
	cpi.Opts.ControlPlaneOnly = true
	cpi.Opts.InfraOnly = true
	cpi.Opts.AuthEnabled = true
	require.NoError(t, CreateGenesisControlPlane(cpi))

	threeportConfig, _, err := GetThreeportConfig("")
	require.NoError(t, err)
	saved, err := threeportConfig.GetControlPlaneConfig("split")
	require.NoError(t, err)
	assert.Equal(t, []string{"203.0.113.7/32"}, saved.K8sApiAllowedCidrs)
	assert.True(t, saved.PrivateK8sApi)
	assert.Equal(t, []string{"198.51.100.20/32"}, saved.EgressCidrs)
	assert.Equal(t, "https://127.0.0.1:6443", saved.KubeAPI.APIEndpoint, "the run did write the config")
}

// TestTptctlUpRecordsTheKubernetesApiAccessItWasGiven covers the tptctl
// config entry of a new install: it records --private-k8s-api and
// --k8s-api-allowed-cidrs, for a later --control-plane-only run and for a
// reinstall to register the cluster with.
func TestTptctlUpRecordsTheKubernetesApiAccessItWasGiven(t *testing.T) {
	kubeconfig := useTestThreeportConfig(t)

	cpi := threeport.NewInstaller()
	cpi.Opts.ControlPlaneName = "split"
	cpi.Opts.InfraProvider = v0.KubernetesRuntimeInfraProviderKind
	cpi.Opts.KubeconfigPath = kubeconfig
	cpi.Opts.ControlPlaneOnly = true
	cpi.Opts.InfraOnly = true
	cpi.Opts.AuthEnabled = true
	cpi.Opts.PrivateK8sApi = true
	cpi.Opts.K8sApiAllowedCidrs = []string{"10.8.0.0/16"}
	require.NoError(t, CreateGenesisControlPlane(cpi))

	threeportConfig, _, err := GetThreeportConfig("")
	require.NoError(t, err)
	saved, err := threeportConfig.GetControlPlaneConfig("split")
	require.NoError(t, err)
	assert.True(t, saved.PrivateK8sApi)
	assert.Equal(t, []string{"10.8.0.0/16"}, saved.K8sApiAllowedCidrs)
}

// TestAPrivateGkeControlPlaneIsRegisteredPrivate covers the
// GcpGkeKubernetesRuntimeDefinition tptctl up registers for the control
// plane's own cluster: it carries PrivateApi along with AllowedCidrs, so the
// API checks a private cluster's list against the private limit and the
// definition says what the cluster is.  A public cluster's definition is
// sent without the field, as before.
func TestAPrivateGkeControlPlaneIsRegisteredPrivate(t *testing.T) {
	for _, private := range []bool{true, false} {
		t.Run(fmt.Sprintf("private=%t", private), func(t *testing.T) {
			var sentDefinition map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					var body map[string]any
					assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					if r.URL.Path == v0.PathGcpGkeKubernetesRuntimeDefinitions {
						sentDefinition = body
					}
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, `{"Data":[{"ID":1}]}`)
					return
				}
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"Data":[]}`)
			}))
			defer server.Close()

			cpi := threeport.NewInstaller()
			cpi.Opts.ControlPlaneName = "private"
			cpi.Opts.ControlPlaneOnly = true
			var infra provider.KubernetesRuntimeInfra = &provider.KubernetesRuntimeInfraGKE{
				ProjectID:              "some-project",
				Region:                 "us-central1",
				WorkerNodeInitialCount: 1,
				PrivateApi:             private,
				AllowedCidrs:           []string{"10.8.0.0/16"},
			}
			require.NoError(t, ConfigureControlPlaneWithGkeConfig(
				cpi,
				&Uninstaller{cpi: cpi},
				server.Client(),
				strings.TrimPrefix(server.URL, "http://"),
				&v0.KubernetesRuntimeDefinition{Common: v0.Common{ID: util.Ptr(uint(1))}},
				&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: util.Ptr(uint(1))}},
				&infra,
			))

			require.NotNil(t, sentDefinition)
			assert.Equal(t, []any{"10.8.0.0/16"}, sentDefinition["AllowedCidrs"])
			if private {
				assert.Equal(t, true, sentDefinition["PrivateApi"])
			} else {
				assert.NotContains(t, sentDefinition, "PrivateApi")
			}
		})
	}
}
