package gcp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestGkeDeleteIsRefusedWhileItsDefinitionIsProtected covers the check
// the delete makes before destroying anything: it reads the GKE
// definition of the instance again, and refuses while DeletionProtection is
// set.
func TestGkeDeleteIsRefusedWhileItsDefinitionIsProtected(t *testing.T) {
	for _, protection := range []*bool{util.Ptr(true), util.Ptr(false), nil} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var object any = api_v0.GcpGkeKubernetesRuntimeInstance{
				Common:                              api_v0.Common{ID: util.Ptr(uint(7))},
				GcpGkeKubernetesRuntimeDefinitionID: util.Ptr(uint(4)),
			}
			if strings.Contains(r.URL.Path, "definitions") {
				object = api_v0.GcpGkeKubernetesRuntimeDefinition{
					Common:             api_v0.Common{ID: util.Ptr(uint(4))},
					Definition:         api_v0.Definition{Name: util.Ptr("prod")},
					DeletionProtection: protection,
				}
			}
			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
				Data:   []apiserver_lib.Object{object},
			})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
		lifecycle := &gkeLifecycle{
			r:          &controller.Reconciler{APIClient: &http.Client{}, APIServer: strings.TrimPrefix(srv.URL, "http://")},
			instanceID: 7,
		}

		err := lifecycle.DeletionRefusal()
		srv.Close()
		if protection == nil || !*protection {
			assert.NoError(t, err)
			continue
		}
		var protected *api_v0.DeletionProtectedError
		require.True(t, errors.As(err, &protected), "%v", err)
		assert.Equal(t, "gcp-gke-kubernetes-runtime-definition", protected.DefinitionType)
		assert.Equal(t, "prod", protected.DefinitionName)
	}
}
