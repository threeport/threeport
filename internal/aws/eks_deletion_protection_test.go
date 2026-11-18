package aws

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
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestEksDeleteIsRefusedWhileItsDefinitionIsProtected covers the check
// the delete makes before destroying anything: it reads the EKS
// definition of the instance again, and refuses while DeletionProtection is
// set.
func TestEksDeleteIsRefusedWhileItsDefinitionIsProtected(t *testing.T) {
	for _, protection := range []*bool{util.Ptr(true), util.Ptr(false), nil} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var object any = v0.AwsEksKubernetesRuntimeInstance{
				Common:                              v0.Common{ID: util.Ptr(uint(7))},
				AwsEksKubernetesRuntimeDefinitionID: util.Ptr(uint(4)),
			}
			if strings.Contains(r.URL.Path, "definitions") {
				object = v0.AwsEksKubernetesRuntimeDefinition{
					Common:             v0.Common{ID: util.Ptr(uint(4))},
					Definition:         v0.Definition{Name: util.Ptr("prod")},
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
		lifecycle := &eksLifecycle{
			r:          &controller.Reconciler{APIClient: &http.Client{}, APIServer: strings.TrimPrefix(srv.URL, "http://")},
			instanceID: 7,
		}

		err := lifecycle.DeletionRefusal()
		srv.Close()
		if protection == nil || !*protection {
			assert.NoError(t, err)
			continue
		}
		var protected *v0.DeletionProtectedError
		require.True(t, errors.As(err, &protected), "%v", err)
		assert.Equal(t, "aws-eks-kubernetes-runtime-definition", protected.DefinitionType)
		assert.Equal(t, "prod", protected.DefinitionName)
	}
}
