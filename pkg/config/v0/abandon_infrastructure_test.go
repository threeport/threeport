package v0

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestRuntimeInstanceConfig_AbandonInfrastructure covers the override reaching
// the API: a PATCH to the object's own path that carries only the flag set
// true. Anything more would write fields the controller owns, and anything less
// leaves a held delete refused.
func TestRuntimeInstanceConfig_AbandonInfrastructure(t *testing.T) {
	instanceID := uint(7)
	name := "dev-20"

	cases := []struct {
		name string
		// existing is the object the by-name lookup returns
		existing apiserver_lib.Object
		// abandon calls the method under test
		abandon func(apiClient *http.Client, apiEndpoint string) error
		// patched reports the flag in the decoded PATCH body, nil when absent
		patched func(body []byte) *bool
	}{
		{
			name: "gcp gke",
			existing: api_v0.GcpGkeKubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &instanceID},
				Instance: api_v0.Instance{Name: &name},
			},
			abandon: func(apiClient *http.Client, apiEndpoint string) error {
				c := GcpGkeKubernetesRuntimeInstanceConfig{
					GcpGkeKubernetesRuntimeInstance: GcpGkeKubernetesRuntimeInstanceValues{Name: &name},
				}
				return c.AbandonInfrastructure(apiClient, apiEndpoint)
			},
			patched: func(body []byte) *bool {
				var o api_v0.GcpGkeKubernetesRuntimeInstance
				require.NoError(t, json.Unmarshal(body, &o))
				return o.AbandonInfrastructure
			},
		},
		{
			name: "aws eks",
			existing: api_v0.AwsEksKubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &instanceID},
				Instance: api_v0.Instance{Name: &name},
			},
			abandon: func(apiClient *http.Client, apiEndpoint string) error {
				c := AwsEksKubernetesRuntimeInstanceConfig{
					AwsEksKubernetesRuntimeInstance: AwsEksKubernetesRuntimeInstanceValues{Name: &name},
				}
				return c.AbandonInfrastructure(apiClient, apiEndpoint)
			},
			patched: func(body []byte) *bool {
				var o api_v0.AwsEksKubernetesRuntimeInstance
				require.NoError(t, json.Unmarshal(body, &o))
				return o.AbandonInfrastructure
			},
		},
		{
			name: "oci oke",
			existing: api_v0.OciOkeKubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &instanceID},
				Instance: api_v0.Instance{Name: &name},
			},
			abandon: func(apiClient *http.Client, apiEndpoint string) error {
				c := OciOkeKubernetesRuntimeInstanceConfig{
					OciOkeKubernetesRuntimeInstance: OciOkeKubernetesRuntimeInstanceValues{Name: &name},
				}
				return c.AbandonInfrastructure(apiClient, apiEndpoint)
			},
			patched: func(body []byte) *bool {
				var o api_v0.OciOkeKubernetesRuntimeInstance
				require.NoError(t, json.Unmarshal(body, &o))
				return o.AbandonInfrastructure
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var patchBody []byte
			var patchPath string
			var methods []string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)

				if r.Method == http.MethodPatch {
					patchPath = r.URL.Path
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					patchBody = body
				}

				resp, err := json.Marshal(apiserver_lib.Response{
					Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
					Data:   []apiserver_lib.Object{tc.existing},
				})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			err := tc.abandon(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"))
			require.NoError(t, err)

			// look up by name, then patch; never a replace or a delete
			assert.Equal(t, []string{http.MethodGet, http.MethodPatch}, methods)

			abandon := tc.patched(patchBody)
			require.NotNil(t, abandon, "the patch must carry the flag")
			assert.True(t, *abandon)
			assert.True(t, strings.HasSuffix(patchPath, "/7"), "the patch must address the instance: %s", patchPath)

			// nothing the controller owns rides along in the patch
			var fields map[string]any
			require.NoError(t, json.Unmarshal(patchBody, &fields))
			for _, owned := range []string{
				"ResourceInventory", "CreationConfirmed", "DeletionScheduled", "Reconciled",
			} {
				assert.NotContains(t, fields, owned)
			}
		})
	}
}
