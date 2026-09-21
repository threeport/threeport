package v0

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetThreeportControlPlaneKubernetesRuntimeInstance_NoMatch covers the
// empty-result path. The filter answers 200 with an empty Data rather than
// 404, so GetResponse returns no error and the caller reaches the index. That
// used to panic with "index out of range [0] with length 0", which surfaced as
// a crashed `tptctl down` against a control plane whose runtime instance row
// was missing — the exact state a caller is trying to tear down.
func TestGetThreeportControlPlaneKubernetesRuntimeInstance_NoMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Meta":{"ObjectCount":0},"Data":[],"Status":{"code":200,"message":"OK"}}`))
	}))
	defer server.Close()

	instance, err := GetThreeportControlPlaneKubernetesRuntimeInstance(server.Client(), strings.TrimPrefix(server.URL, "http://"))

	require.Error(t, err, "an empty result must surface as an error, not a panic")
	assert.Contains(t, err.Error(), "no kubernetes runtime instance found")
	assert.NotNil(t, instance, "the caller is handed a usable zero value alongside the error")
}

// TestGetThreeportControlPlaneKubernetesRuntimeInstance_MultipleMatches covers
// the other ambiguous case: more than one runtime instance flagged as the
// control plane host means the caller cannot pick one safely.
func TestGetThreeportControlPlaneKubernetesRuntimeInstance_MultipleMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Meta":{"ObjectCount":2},"Data":[{"Name":"a"},{"Name":"b"}],"Status":{"code":200,"message":"OK"}}`))
	}))
	defer server.Close()

	_, err := GetThreeportControlPlaneKubernetesRuntimeInstance(server.Client(), strings.TrimPrefix(server.URL, "http://"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple kubernetes runtime instances")
}

// TestGetThreeportControlPlaneKubernetesRuntimeInstance_SingleMatch is the
// happy path, so the guards above cannot pass by rejecting everything.
func TestGetThreeportControlPlaneKubernetesRuntimeInstance_SingleMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Meta":{"ObjectCount":1},"Data":[{"Name":"threeport-dev-0"}],"Status":{"code":200,"message":"OK"}}`))
	}))
	defer server.Close()

	instance, err := GetThreeportControlPlaneKubernetesRuntimeInstance(server.Client(), strings.TrimPrefix(server.URL, "http://"))

	require.NoError(t, err)
	require.NotNil(t, instance.Name)
	assert.Equal(t, "threeport-dev-0", *instance.Name)
}
