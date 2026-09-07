package v0

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serializerPayload mirrors the shape of a threeport api object: pointer
// fields, most of them nil on any given response.
type serializerPayload struct {
	ID            *uint
	Name          *string
	InfraProvider *string
	Instances     []*string
}

// newSerializerContext builds an echo context wired to the api server's
// serializer, matching how the generated main assigns it.
func newSerializerContext() (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	e.JSONSerializer = NewJSONSerializer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// TestJSONSerializer_OmitsAbsentFields is the contract the removed
// json:",omitempty" struct tag used to provide on the response side. Without
// it, every nil pointer would be spelled out as null, which is a wire change
// for any consumer that distinguishes an absent key from a null one.
func TestJSONSerializer_OmitsAbsentFields(t *testing.T) {
	c, rec := newSerializerContext()
	name := "threeport-dev-0"

	require.NoError(t, c.JSON(http.StatusOK, serializerPayload{Name: &name}))

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	assert.Equal(t, name, got["Name"])
	for key, value := range got {
		assert.NotNil(t, value, "field %q serialized as JSON null instead of being omitted", key)
	}
	assert.NotContains(t, got, "ID")
	assert.NotContains(t, got, "InfraProvider")
	assert.NotContains(t, got, "Instances")
}

// TestJSONSerializer_MatchesDefaultFraming pins the framing down to the byte.
// echo's default serializer writes through json.Encoder, which terminates the
// document with a newline; MarshalWrite does not, so the serializer adds it
// back. A response that silently loses its trailing newline is the kind of
// difference no functional test would catch.
func TestJSONSerializer_MatchesDefaultFraming(t *testing.T) {
	c, rec := newSerializerContext()
	name := "threeport-dev-0"

	require.NoError(t, c.JSON(http.StatusOK, serializerPayload{Name: &name}))

	assert.Equal(t, "{\"Name\":\"threeport-dev-0\"}\n", rec.Body.String())
}

// TestJSONSerializer_Indent covers the JSONPretty path, which passes a
// non-empty indent.
func TestJSONSerializer_Indent(t *testing.T) {
	c, rec := newSerializerContext()
	name := "threeport-dev-0"

	require.NoError(t, c.JSONPretty(http.StatusOK, serializerPayload{Name: &name}, "  "))

	assert.Equal(t, "{\n  \"Name\": \"threeport-dev-0\"\n}\n", rec.Body.String())
}

// TestJSONSerializer_KeepsExplicitEmptyValues covers the deliberate difference
// from the old omitempty tag. A non-nil pointer to "" and an initialized empty
// slice are both present values, so they reach the client rather than being
// silently dropped.
func TestJSONSerializer_KeepsExplicitEmptyValues(t *testing.T) {
	c, rec := newSerializerContext()
	empty := ""

	require.NoError(t, c.JSON(http.StatusOK, serializerPayload{
		Name:      &empty,
		Instances: []*string{},
	}))

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	require.Contains(t, got, "Name")
	assert.Equal(t, "", got["Name"])
	require.Contains(t, got, "Instances")
	assert.Empty(t, got["Instances"])
}

// TestJSONSerializer_DeserializeUnchanged confirms the request side still
// answers 400 on a malformed body. OmitZeroStructFields is a marshal option,
// so reading must behave exactly as echo's default does.
func TestJSONSerializer_DeserializeUnchanged(t *testing.T) {
	e := echo.New()
	e.JSONSerializer = NewJSONSerializer()
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader([]byte(`{"ID":"not-a-number"}`)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())

	var payload serializerPayload
	err := c.Bind(&payload)

	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok, "a malformed body must surface as an echo.HTTPError")
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}
