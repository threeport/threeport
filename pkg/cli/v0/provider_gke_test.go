package v0

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestExistingOrCreateGcpProviderReusesNamedRow covers a retry that would
// otherwise POST a duplicate provider.
func TestExistingOrCreateGcpProviderReusesNamedRow(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Data":[{"ID":4,"Name":%q}]}`, r.URL.Query().Get("name"))
	}))
	defer server.Close()

	got, err := existingOrCreateGcpProvider(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.GcpProvider{Name: util.Ptr("default")},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(4), *got.ID)
}

// TestExistingOrCreateGcpGkeKubernetesRuntimeDefinitionReusesNamedRow covers
// a retry of the GKE definition create.
func TestExistingOrCreateGcpGkeKubernetesRuntimeDefinitionReusesNamedRow(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Data":[{"ID":5,"Name":%q}]}`, r.URL.Query().Get("name"))
	}))
	defer server.Close()

	got, err := existingOrCreateGcpGkeKubernetesRuntimeDefinition(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.GcpGkeKubernetesRuntimeDefinition{
			Definition: v0.Definition{Name: util.Ptr("threeport-dev")},
		},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(5), *got.ID)
}

// TestExistingOrCreateGcpGkeKubernetesRuntimeInstanceReusesNamedRow covers a
// retry of the GKE instance create.
func TestExistingOrCreateGcpGkeKubernetesRuntimeInstanceReusesNamedRow(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Data":[{"ID":6,"Name":%q}]}`, r.URL.Query().Get("name"))
	}))
	defer server.Close()

	got, err := existingOrCreateGcpGkeKubernetesRuntimeInstance(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.GcpGkeKubernetesRuntimeInstance{
			Instance: v0.Instance{Name: util.Ptr("threeport-dev")},
		},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(6), *got.ID)
}
