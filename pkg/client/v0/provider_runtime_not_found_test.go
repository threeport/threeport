package v0

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// TestProviderRuntimeLookups_EmptyResultIsObjectNotFound covers the lookups of a
// provider-specific runtime record by its kubernetes runtime definition or
// instance. They filter a list, so a missing record is a 200 with an empty
// Data rather than a 404. Callers decide "already gone" with
// errors.Is(err, ErrObjectNotFound), so the error has to wrap it: a plain
// error left the kubernetes runtime instance delete requeueing forever once
// its provider record had been deleted.
func TestProviderRuntimeLookups_EmptyResultIsObjectNotFound(t *testing.T) {
	server := emptyDataServer(t)
	defer server.Close()

	client := server.Client()
	addr := apiAddr(server)

	lookups := []struct {
		name string
		call func() error
	}{
		{"gcp gke definition", func() error {
			_, err := GetGcpGkeKubernetesRuntimeDefinitionByK8sRuntimeDef(client, addr, 1)
			return err
		}},
		{"gcp gke instance", func() error {
			_, err := GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(client, addr, 1)
			return err
		}},
		{"aws eks definition", func() error {
			_, err := GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(client, addr, 1)
			return err
		}},
		{"aws eks instance", func() error {
			_, err := GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(client, addr, 1)
			return err
		}},
		{"oci oke definition", func() error {
			_, err := GetOciOkeKubernetesRuntimeDefinitionByK8sRuntimeDef(client, addr, 1)
			return err
		}},
		{"oci oke instance", func() error {
			_, err := GetOciOkeKubernetesRuntimeInstanceByK8sRuntimeInst(client, addr, 1)
			return err
		}},
	}

	for _, lookup := range lookups {
		t.Run(lookup.name, func(t *testing.T) {
			err := lookup.call()
			require.Error(t, err)
			assert.ErrorIs(t, err, client_lib.ErrObjectNotFound)
			assert.Contains(t, err.Error(), "no object found with ID 1")
		})
	}
}

// TestProviderRuntimeLookups_FoundIsNotAnError is the happy path, so the
// assertions above cannot pass by rejecting everything.
func TestProviderRuntimeLookups_FoundIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Meta":{"ObjectCount":1},"Data":[{"ID":7}],"Status":{"code":200,"message":"OK"}}`))
	}))
	defer server.Close()

	instance, err := GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(server.Client(), apiAddr(server), 1)
	require.NoError(t, err)
	require.NotNil(t, instance.ID)
	assert.Equal(t, uint(7), *instance.ID)
}
