package kubernetesruntime

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/machinetest"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestTheClusterVersionIsReadWithTheStoredToken covers reading an EKS
// cluster's version for its autoscaler.  The reconciler's copy of the runtime
// may hold a token a client refreshed, unencrypted, so the version is read with
// the runtime as the API stores it, whose token is encrypted.
func TestTheClusterVersionIsReadWithTheStoredToken(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	encryptedToken, err := encryption.Encrypt(key, "cluster-token")
	require.NoError(t, err)

	cluster := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cluster-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"major": "1", "minor": "36+", "gitVersion": "v1.36.5-eks-1a2b3c4"})
	}))
	t.Cleanup(cluster.Close)
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cluster.Certificate().Raw}))

	api := machinetest.NewAPIStub(t)
	api.Mux.HandleFunc(fmt.Sprintf("%s/%d", v0.PathKubernetesRuntimeInstances, 9), func(w http.ResponseWriter, r *http.Request) {
		machinetest.WriteResponse(t, w, http.StatusOK, []apiserver_lib.Object{v0.KubernetesRuntimeInstance{
			Common:          v0.Common{ID: util.Ptr(uint(9))},
			Instance:        v0.Instance{Name: util.Ptr("workload")},
			APIEndpoint:     util.Ptr(cluster.URL),
			CACertificate:   &ca,
			ConnectionToken: &encryptedToken,
		}})
	})
	r := &controller.Reconciler{APIClient: api.Client, APIServer: api.Addr, EncryptionKey: key}

	version, err := clusterMinorVersion(r, 9, false)
	require.NoError(t, err)
	assert.Equal(t, "1.36", version)

	// the copy a client refreshed holds the token unencrypted, which a
	// REST config built from it again can't decrypt
	refreshed := &v0.KubernetesRuntimeInstance{
		APIEndpoint:     util.Ptr(cluster.URL),
		CACertificate:   &ca,
		ConnectionToken: util.Ptr("cluster-token"),
	}
	_, err = kube.GetDiscoveryClient(refreshed, false, r.APIClient, r.APIServer, key)
	assert.ErrorContains(t, err, "failed to decrypt")
}
