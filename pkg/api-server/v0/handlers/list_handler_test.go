package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
)

// newListRequest drives a generated list handler the way the router does: the
// strict query binder registered on the echo instance, and the context wrapped
// so the handler's type assertion to CustomContext succeeds. No database is
// involved because a bind failure is decided before the handler queries.
func newListRequest(target string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	return &apiserver_lib.CustomContext{Context: e.NewContext(req, rec)}, rec
}

// TestListHandlerRejectsUnknownQueryParamWith400 asserts the layer that
// actually sets the status, not the binder underneath it. A typo'd filter is
// client error, and answering 500 tells the caller to retry something that will
// never succeed. The binder test proves the error is produced; this proves the
// handler turns it into the right status.
func TestListHandlerRejectsUnknownQueryParamWith400(t *testing.T) {
	c, rec := newListRequest("/v0/kubernetes-workload-definitions?nmae=my-app")
	h := Handler{Logger: zap.NewNop()}

	require.NoError(t, h.GetKubernetesWorkloadDefinitions(c))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "nmae", "the response names the parameter that was rejected")
}

// TestListHandlerRejectsBadLimitWith400 asserts the same for a limit the
// pagination params reject. A non-positive limit reaches CockroachDB as a
// negative LIMIT and surfaces its sqlstate to the client, so it has to be
// refused before the query is built.
func TestListHandlerRejectsBadLimitWith400(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"zero limit", "/v0/kubernetes-workload-definitions?limit=0"},
		{"negative limit", "/v0/kubernetes-workload-definitions?limit=-5"},
		{"limit over the maximum", "/v0/kubernetes-workload-definitions?limit=100000"},
		{"unparseable limit", "/v0/kubernetes-workload-definitions?limit=lots"},
		{"unparseable cursor", "/v0/kubernetes-workload-definitions?cursor=first"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, rec := newListRequest(test.target)
			h := Handler{Logger: zap.NewNop()}

			require.NoError(t, h.GetKubernetesWorkloadDefinitions(c))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
