package oci

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestOkeLifecycle_GetReconciliation_AbandonInfrastructure covers the operator
// override reaching the shared delete state machine. A snapshot that drops it
// leaves a held delete refused no matter what the operator sets.
func TestOkeLifecycle_GetReconciliation_AbandonInfrastructure(t *testing.T) {
	cases := []struct {
		name    string
		abandon *bool
		want    bool
	}{
		{name: "unset is not abandoned", abandon: nil, want: false},
		{name: "false is not abandoned", abandon: util.Ptr(false), want: false},
		{name: "true is abandoned", abandon: util.Ptr(true), want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instanceID := uint(7)
			instance := v0.OciOkeKubernetesRuntimeInstance{
				Common:                v0.Common{ID: &instanceID},
				AbandonInfrastructure: tc.abandon,
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := json.Marshal(apiserver_lib.Response{
					Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
					Data:   []apiserver_lib.Object{instance},
				})
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			lifecycle := &okeLifecycle{
				r: &controller.Reconciler{
					APIClient: &http.Client{},
					APIServer: strings.TrimPrefix(srv.URL, "http://"),
				},
				instanceID: instanceID,
			}

			snap, err := lifecycle.GetReconciliation()
			require.NoError(t, err)
			assert.Equal(t, tc.want, snap.AbandonInfrastructure)
		})
	}
}
