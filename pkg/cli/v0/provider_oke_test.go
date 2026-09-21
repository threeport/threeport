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

// TestEnsureOciProviderReusesNamedRow covers a retry that would
// otherwise POST a duplicate provider.
func TestEnsureOciProviderReusesNamedRow(t *testing.T) {
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

	got, err := ensureOciProvider(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.OciProvider{Name: util.Ptr("svc")},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(4), *got.ID)
}

// TestEnsureOciProviderCreatesWhenMissing covers a first install.
func TestEnsureOciProviderCreatesWhenMissing(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"Data":[{"ID":8,"Name":"svc"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Data":[]}`))
	}))
	defer server.Close()

	got, err := ensureOciProvider(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.OciProvider{Name: util.Ptr("svc")},
	)

	require.NoError(err)
	require.Equal(1, posts)
	require.Equal(uint(8), *got.ID)
}

// TestEnsureOciOkeKubernetesRuntimeDefinitionReusesNamedRow covers
// a retry of the OKE definition create.
func TestEnsureOciOkeKubernetesRuntimeDefinitionReusesNamedRow(t *testing.T) {
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

	got, err := ensureOciOkeKubernetesRuntimeDefinition(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.OciOkeKubernetesRuntimeDefinition{
			Definition: v0.Definition{Name: util.Ptr("threeport-dev")},
		},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(5), *got.ID)
}

// TestEnsureOciOkeKubernetesRuntimeInstanceReusesNamedRow covers a
// retry of the OKE instance create.
func TestEnsureOciOkeKubernetesRuntimeInstanceReusesNamedRow(t *testing.T) {
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

	got, err := ensureOciOkeKubernetesRuntimeInstance(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.OciOkeKubernetesRuntimeInstance{
			Instance: v0.Instance{Name: util.Ptr("threeport-dev")},
		},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(6), *got.ID)
}
