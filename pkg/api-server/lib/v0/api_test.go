package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bindPayload mirrors the shape of a threeport api patch payload:
// a uint field that clients may accidentally submit as a JSON string.
type bindPayload struct {
	KubernetesRuntimeDefinitionID uint
}

// newHelperContext builds an echo context wrapping a JSON-body request,
// matching the shape the generated PATCH handlers hand to c.Bind.
func newHelperContext(body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// TestResponseStatusBindErr_UnmarshalTypeErrorReturns400 asserts a JSON
// string bound to a uint field surfaces as 400, with the unmarshal message
// in the response body so the client can see which field went wrong.
func TestResponseStatusBindErr_UnmarshalTypeErrorReturns400(t *testing.T) {
	// arrange a request body that fails Bind on the uint field
	body := []byte(`{"KubernetesRuntimeDefinitionID":"not-a-number"}`)
	c, rec := newHelperContext(body)

	// action: mirror the handler flow: c.Bind then pass the error to the helper
	var payload bindPayload
	bindErr := c.Bind(&payload)
	require.Error(t, bindErr, "c.Bind must fail on a string-to-uint mismatch")
	require.NoError(t, ResponseStatusBindErr(c, nil, bindErr, "KubernetesRuntimeInstance"))

	// assert HTTP status is 400, not 500
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// assert the body carries the unmarshal message and nothing else. An
	// exact match is what makes this assertion able to fail: a Contains on
	// the same substring passed both before and after the body stopped
	// repeating the status code and naming Go types
	var resp Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, http.StatusBadRequest, resp.Status.Code)
	assert.Equal(t,
		"Unmarshal type error: expected=uint, got=string, field=KubernetesRuntimeDefinitionID, offset=47",
		resp.Status.Error)
	assert.NotContains(t, resp.Status.Error, "code=400",
		"the status code belongs in Status.Code, not repeated in the message")
	assert.NotContains(t, resp.Status.Error, "internal=",
		"the wrapped cause is for the log line, not the client")
}

// TestResponseStatusBindErr_PlainErrorFallsBackTo500 confirms non-HTTP
// errors, such as a database or serialization failure that happens to
// flow through this helper, still surface as 500.
func TestResponseStatusBindErr_PlainErrorFallsBackTo500(t *testing.T) {
	// arrange a bare context and hand the helper a plain (non-HTTP) error
	c, rec := newHelperContext(nil)

	// action: pass a non-HTTPError through the helper
	require.NoError(t, ResponseStatusBindErr(c, nil, errors.New("boom"), "KubernetesRuntimeInstance"))

	// assert plain errors are treated as internal failures
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestResponseStatusBindErr_5xxHTTPErrorFallsBackTo500 makes sure a
// server-side echo.HTTPError isn't leaked as a 4xx by the helper: the
// 400-499 range is the only client-error passthrough.
func TestResponseStatusBindErr_5xxHTTPErrorFallsBackTo500(t *testing.T) {
	// arrange an *echo.HTTPError with a 5xx code
	c, rec := newHelperContext(nil)
	httpErr := echo.NewHTTPError(http.StatusBadGateway, "upstream failed")

	// action: pass the 5xx HTTPError through the helper
	require.NoError(t, ResponseStatusBindErr(c, nil, httpErr, "KubernetesRuntimeInstance"))

	// assert 5xx HTTPErrors do not get downgraded to a client status
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
