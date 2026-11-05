package v0

import (
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestTheAllowedCidrsFlagIsCheckedBeforeAnythingIsCreated covers
// --k8s-api-allowed-cidrs: it applies to a cluster tptctl creates on a cloud
// provider, and its blocks are checked against the provider's limit.
func TestTheAllowedCidrsFlagIsCheckedBeforeAnythingIsCreated(t *testing.T) {
	blocks := func(n int) []string {
		cidrs := make([]string, n)
		for i := range cidrs {
			cidrs[i] = fmt.Sprintf("198.51.%d.%d/32", i/256, i%256)
		}
		return cidrs
	}

	cases := []struct {
		name             string
		cidrs            []string
		provider         string
		controlPlaneOnly bool
		wantErr          string
	}{
		{"not set", nil, v0.KubernetesRuntimeInfraProviderEKS, false, ""},
		{"not set on kind", nil, v0.KubernetesRuntimeInfraProviderKind, false, ""},
		{"not set with control plane only", nil, v0.KubernetesRuntimeInfraProviderGKE, true, ""},
		{"set on EKS", []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderEKS, false, ""},
		{"set on GKE", []string{"203.0.113.0/24", "10.8.0.0/16"}, v0.KubernetesRuntimeInfraProviderGKE, false, ""},
		{"set on OKE", []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderOKE, false, ""},
		{"on kind", []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderKind, false, "not supported with infrastructure provider 'kind'"},
		{"with control plane only", []string{"203.0.113.7/32"}, v0.KubernetesRuntimeInfraProviderEKS, true, "not supported with --control-plane-only"},
		{"an address without a prefix", []string{"203.0.113.7"}, v0.KubernetesRuntimeInfraProviderEKS, false, `--k8s-api-allowed-cidrs entry "203.0.113.7" is not a CIDR block`},
		{"host bits set", []string{"203.0.113.7/24"}, v0.KubernetesRuntimeInfraProviderGKE, false, `write it as "203.0.113.0/24"`},
		{"EKS at its limit", blocks(40), v0.KubernetesRuntimeInfraProviderEKS, false, ""},
		{"EKS past its limit", blocks(41), v0.KubernetesRuntimeInfraProviderEKS, false, "has 41 entries and eks admits at most 40"},
		{"GKE past its public limit", blocks(51), v0.KubernetesRuntimeInfraProviderGKE, false, "admits at most 50"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateK8sApiAllowedCidrsFlag(c.cidrs, c.provider, c.controlPlaneOnly)
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
