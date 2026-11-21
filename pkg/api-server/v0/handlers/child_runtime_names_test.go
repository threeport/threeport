package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestChildRuntimeNamesOmitsTheHost covers a child that still manages one workload runtime.
func TestChildRuntimeNamesOmitsTheHost(t *testing.T) {
	// the child runs on runtime 1 and also stores a workload runtime
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.RawQuery, "isself=true"):
			_, _ = w.Write([]byte(`{"Data":[{"ID":7,"IsSelf":true,"KubernetesRuntimeInstanceID":1}]}`))
		default:
			_, _ = w.Write([]byte(`{"Data":[{"ID":1,"Name":"host"},{"ID":2,"Name":"workload"}]}`))
		}
	}))
	t.Cleanup(server.Close)

	// the host runtime is not reported
	names, err := childRuntimeNames(&api_v0.ControlPlaneInstance{
		ApiServerEndpoint: util.Ptr(strings.TrimPrefix(server.URL, "http://")),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"workload"}, names)
}
