package v0

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// TestRuntimeInstanceConfig_SetAbandonInfrastructure covers the override
// reaching the API: a PATCH to the object's own path that carries only the
// flag. Anything more would write fields the controller owns, and anything less
// leaves a held delete refused. False must be sent as false, not left out, or a
// failed delete could not switch the override back off.
func TestRuntimeInstanceConfig_SetAbandonInfrastructure(t *testing.T) {
	instanceID := uint(7)
	name := "dev-20"

	cases := []struct {
		name string
		// existing is the object the by-name lookup returns
		existing apiserver_lib.Object
		// abandon calls the method under test
		abandon func(apiClient *http.Client, apiEndpoint string, abandon bool) error
		// patched reports the flag in the decoded PATCH body, nil when absent
		patched func(body []byte) *bool
	}{
		{
			name: "gcp gke",
			existing: api_v0.GcpGkeKubernetesRuntimeInstance{
				Common:   api_v0.Common{ID: &instanceID},
				Instance: api_v0.Instance{Name: &name},
			},
			abandon: func(apiClient *http.Client, apiEndpoint string, abandon bool) error {
				c := GcpGkeKubernetesRuntimeInstanceConfig{
					GcpGkeKubernetesRuntimeInstance: GcpGkeKubernetesRuntimeInstanceValues{Name: &name},
				}
				return c.SetAbandonInfrastructure(apiClient, apiEndpoint, abandon)
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
			abandon: func(apiClient *http.Client, apiEndpoint string, abandon bool) error {
				c := AwsEksKubernetesRuntimeInstanceConfig{
					AwsEksKubernetesRuntimeInstance: AwsEksKubernetesRuntimeInstanceValues{Name: &name},
				}
				return c.SetAbandonInfrastructure(apiClient, apiEndpoint, abandon)
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
			abandon: func(apiClient *http.Client, apiEndpoint string, abandon bool) error {
				c := OciOkeKubernetesRuntimeInstanceConfig{
					OciOkeKubernetesRuntimeInstance: OciOkeKubernetesRuntimeInstanceValues{Name: &name},
				}
				return c.SetAbandonInfrastructure(apiClient, apiEndpoint, abandon)
			},
			patched: func(body []byte) *bool {
				var o api_v0.OciOkeKubernetesRuntimeInstance
				require.NoError(t, json.Unmarshal(body, &o))
				return o.AbandonInfrastructure
			},
		},
	}

	for _, tc := range cases {
		for _, want := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s %t", tc.name, want), func(t *testing.T) {
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

				err := tc.abandon(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"), want)
				require.NoError(t, err)

				// look up by name, then patch; never a replace or a delete
				assert.Equal(t, []string{http.MethodGet, http.MethodPatch}, methods)

				abandon := tc.patched(patchBody)
				require.NotNil(t, abandon, "the patch must carry the flag")
				assert.Equal(t, want, *abandon)
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
}

// TestWaitForInstanceDelete covers the wait after a runtime instance delete is
// accepted: done only when the instance is gone, a timeout the CLI can tell
// apart from a failed request, and an API error never taken for success.
func TestWaitForInstanceDelete(t *testing.T) {
	original := instanceDeleteWait
	t.Cleanup(func() { instanceDeleteWait = original })
	instanceDeleteWait = struct{ attempts, intervalSeconds int }{attempts: 3, intervalSeconds: 0}

	t.Run("gone once the lookup is not found", func(t *testing.T) {
		calls := 0
		err := waitForInstanceDelete("thing", "dev-1", func() error {
			calls++
			if calls < 2 {
				return nil
			}
			return client_lib.ErrObjectNotFound
		})

		require.NoError(t, err)
		assert.Equal(t, 2, calls)
	})

	t.Run("still present times out with the sentinel and a pointer to events", func(t *testing.T) {
		err := waitForInstanceDelete("thing", "dev-1", func() error { return nil })

		require.ErrorIs(t, err, ErrDeleteTimedOut)
		assert.Contains(t, err.Error(), "check its events")
		assert.Contains(t, err.Error(), "dev-1")
	})

	t.Run("an API error is not taken for success", func(t *testing.T) {
		err := waitForInstanceDelete("thing", "dev-1", func() error {
			return errors.New("injected: connection refused")
		})

		require.ErrorIs(t, err, ErrDeleteTimedOut)
		assert.Contains(t, err.Error(), "connection refused")
	})
}

// TestGcpGkeKubernetesRuntimeInstanceConfig_Delete_WaitsForTheController covers
// the delete reporting done only once the controller has removed the instance.
// The API accepting the DELETE is not that: the controller can refuse, and then
// the instance is still there.
func TestGcpGkeKubernetesRuntimeInstanceConfig_Delete_WaitsForTheController(t *testing.T) {
	original := instanceDeleteWait
	t.Cleanup(func() { instanceDeleteWait = original })
	instanceDeleteWait = struct{ attempts, intervalSeconds int }{attempts: 3, intervalSeconds: 0}

	instanceID := uint(7)
	name := "dev-20"
	instance := api_v0.GcpGkeKubernetesRuntimeInstance{
		Common:   api_v0.Common{ID: &instanceID},
		Instance: api_v0.Instance{Name: &name},
	}

	cases := []struct {
		name string
		// goneAfterGets is how many by-name lookups find the instance before
		// it disappears; negative means it never does
		goneAfterGets int
		wantErr       error
	}{
		{name: "controller removes the instance", goneAfterGets: 2},
		{name: "controller refuses the delete", goneAfterGets: -1, wantErr: ErrDeleteTimedOut},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gets := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data := []apiserver_lib.Object{instance}
				if r.Method == http.MethodGet {
					gets++
					if tc.goneAfterGets >= 0 && gets > tc.goneAfterGets {
						data = []apiserver_lib.Object{}
					}
				}

				resp, err := json.Marshal(apiserver_lib.Response{
					Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
					Data:   data,
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

			c := GcpGkeKubernetesRuntimeInstanceConfig{
				GcpGkeKubernetesRuntimeInstance: GcpGkeKubernetesRuntimeInstanceValues{Name: &name},
			}
			_, err := c.Delete(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"))

			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}
