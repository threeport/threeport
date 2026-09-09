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

// newListRequest returns a CustomContext and recorder for a GET of target.
func newListRequest(target string) (*apiserver_lib.CustomContext, *httptest.ResponseRecorder) {
	e := echo.New()
	// install QueryBinder so unknown keys fail the bind
	e.Binder = apiserver_lib.NewQueryBinder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	// wrap in CustomContext so the handler's type assertion succeeds
	return &apiserver_lib.CustomContext{Context: e.NewContext(req, rec)}, rec
}

// TestListHandlerRejectsUnknownQueryParamWith400 rejects nmae with 400, not
// 500, and names the unknown key in the body.
func TestListHandlerRejectsUnknownQueryParamWith400(t *testing.T) {
	// GET with a typo of name
	c, rec := newListRequest("/v0/kubernetes-workload-definitions?nmae=my-app")
	// handler with no DB; 400 paths return before the query
	h := Handler{Logger: zap.NewNop()}

	// run the list handler
	require.NoError(t, h.GetKubernetesWorkloadDefinitions(c))

	// check 400 names the unknown key
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "nmae", "the response names the parameter that was rejected")
}

// TestListHandlerRejectsBadLimitWith400 rejects a non-positive, over-max, or
// unparseable limit or cursor with 400.
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
			// GET the bad query
			c, rec := newListRequest(test.target)
			h := Handler{Logger: zap.NewNop()}

			// run the list handler
			require.NoError(t, h.GetKubernetesWorkloadDefinitions(c))

			// check 400
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
