package v0

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBindContext spins up an Echo context wrapping a request shaped
// like the production handlers receive. Each test calls Bind on the
// returned context so the assertion surface mirrors real handler use.
func newBindContext(method, target string, body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var reader = bytes.NewReader(body)
	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// bindTestPayload mirrors the shape of a typical threeport api filter
// struct: embedded base types and pointer fields. The point of the
// QueryBinder is to derive each param key from strings.ToLower(field
// name), so the lack of `query:` tags here is intentional.
type bindTestCommon struct {
	ID *uint
}

type bindTestFilter struct {
	bindTestCommon
	Name        *string
	Count       *uint
	Active      *bool
	Description string
}

// TestQueryBinder_PrimitivesAndPointers verifies the lowercased
// field-name convention covers strings, pointer strings, pointer
// uints and pointer bools for both top-level and embedded fields.
func TestQueryBinder_PrimitivesAndPointers(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/?id=42&name=foo&count=7&active=true&description=hello", nil)
	var filter bindTestFilter
	require.NoError(t, NewQueryBinder().Bind(&filter, c))

	require.NotNil(t, filter.ID)
	assert.Equal(t, uint(42), *filter.ID)
	require.NotNil(t, filter.Name)
	assert.Equal(t, "foo", *filter.Name)
	require.NotNil(t, filter.Count)
	assert.Equal(t, uint(7), *filter.Count)
	require.NotNil(t, filter.Active)
	assert.True(t, *filter.Active)
	assert.Equal(t, "hello", filter.Description)
}

// TestQueryBinder_MissingParamLeavesFieldZero confirms missing params
// don't reset existing values; the binder skips fields whose key isn't
// in the request, matching echo's default-binder behavior.
func TestQueryBinder_MissingParamLeavesFieldZero(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/?name=just-name", nil)
	var filter bindTestFilter
	require.NoError(t, NewQueryBinder().Bind(&filter, c))

	require.NotNil(t, filter.Name)
	assert.Equal(t, "just-name", *filter.Name)
	assert.Nil(t, filter.ID, "ID stays nil when no id param present")
	assert.Nil(t, filter.Count, "Count stays nil when no count param present")
}

// TestQueryBinder_UnknownParamIgnored verifies extra query params that
// don't correspond to any field are silently dropped rather than
// erroring out - matches the permissive default binder.
func TestQueryBinder_UnknownParamIgnored(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/?name=keep&irrelevant=junk", nil)
	var filter bindTestFilter
	require.NoError(t, NewQueryBinder().Bind(&filter, c))

	require.NotNil(t, filter.Name)
	assert.Equal(t, "keep", *filter.Name)
}

// TestQueryBinder_MalformedValueError exercises the path where the
// query param value can't be parsed into the target field's type.
// Callers (api handlers) translate this into a 400 to the client.
func TestQueryBinder_MalformedValueError(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/?count=not-a-number", nil)
	var filter bindTestFilter
	err := NewQueryBinder().Bind(&filter, c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

// TestQueryBinder_QuerySkippedOnPost confirms the binder routes
// non-read methods to the body decoder. POST with a JSON body should
// populate the struct from JSON, ignoring query params entirely - the
// convention mirrors echo's default binder so threeport handlers that
// rely on "GET = query, POST = body" keep working.
func TestQueryBinder_QuerySkippedOnPost(t *testing.T) {
	body := []byte(`{"Name":"from-body","Count":99}`)
	c, _ := newBindContext(http.MethodPost, "/?name=from-query", body)
	var filter bindTestFilter
	require.NoError(t, NewQueryBinder().Bind(&filter, c))

	require.NotNil(t, filter.Name)
	assert.Equal(t, "from-body", *filter.Name, "POST takes input from body, not query")
	require.NotNil(t, filter.Count)
	assert.Equal(t, uint(99), *filter.Count)
}

// TestQueryBinder_DeleteUsesQuery confirms DELETE binds query params
// (some threeport delete-by-name endpoints filter via the query string).
func TestQueryBinder_DeleteUsesQuery(t *testing.T) {
	c, _ := newBindContext(http.MethodDelete, "/?name=to-delete", nil)
	var filter bindTestFilter
	require.NoError(t, NewQueryBinder().Bind(&filter, c))

	require.NotNil(t, filter.Name)
	assert.Equal(t, "to-delete", *filter.Name)
}

// TestQueryBinder_RejectsNonPointerTarget ensures the binder fails
// loudly when handed a value rather than a pointer-to-struct; silently
// returning would hide a real handler bug.
func TestQueryBinder_RejectsNonPointerTarget(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/?name=foo", nil)
	var filter bindTestFilter
	err := NewQueryBinder().Bind(filter, c) // note: value, not pointer
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-nil pointer")
}

// TestQueryBinder_EmptyQueryString is the trivial happy path with no
// params at all. Bind should succeed and leave the target untouched.
func TestQueryBinder_EmptyQueryString(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/", nil)
	var filter bindTestFilter
	require.NoError(t, NewQueryBinder().Bind(&filter, c))
	assert.Nil(t, filter.ID)
	assert.Nil(t, filter.Name)
}

// bindTestFloatFilter covers the float32/float64 value-parsing paths in
// setFieldFromString. No threeport api type uses floats today, but the
// binder supports them and a regression should be caught.
type bindTestFloatFilter struct {
	Threshold *float64
	Ratio     float32
}

// TestQueryBinder_FloatScalars exercises the float32/float64 branches of
// setFieldFromString. The query keys are the usual lowercased field names;
// what's being tested is that the raw string VALUES parse correctly into
// pointer-float64 and value-float32 fields via strconv.ParseFloat.
//
// Example: ?threshold=0.25 binds *Threshold = 0.25; ?ratio=1.5 binds
// Ratio = 1.5.
func TestQueryBinder_FloatScalars(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/?threshold=0.25&ratio=1.5", nil)
	var filter bindTestFloatFilter
	require.NoError(t, NewQueryBinder().Bind(&filter, c))

	require.NotNil(t, filter.Threshold)
	assert.InDelta(t, 0.25, *filter.Threshold, 0.0001)
	assert.InDelta(t, float32(1.5), filter.Ratio, 0.0001)
}

// TestQueryBinder_MalformedFloatValue mirrors the malformed-int test for
// the float branch of setFieldFromString. The query value isn't parseable
// as a float so strconv.ParseFloat returns an error, which the binder
// wraps with the param name.
//
// Example: ?threshold=not-a-float surfaces an error citing "threshold".
func TestQueryBinder_MalformedFloatValue(t *testing.T) {
	c, _ := newBindContext(http.MethodGet, "/?threshold=not-a-float", nil)
	var filter bindTestFloatFilter
	err := NewQueryBinder().Bind(&filter, c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "threshold")
}

