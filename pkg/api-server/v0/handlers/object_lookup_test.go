package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
)

// fakeModuleServer returns an httptest server that fakes the module
// CRUD endpoint. handler controls per-test responses; the server URL
// is split into the endpoint host and a captured path prefix so the
// caller can pass them into the lookup functions exactly as the
// real api-server does.
func fakeModuleServer(t *testing.T, handler http.HandlerFunc) (endpoint, host string, restore func()) {
	t.Helper()
	srv := httptest.NewServer(handler)

	// GetResponse prepends "http://" to whatever endpoint we pass, so
	// strip the scheme from the test-server URL to mirror the
	// production endpoint shape (bare DNS, no scheme).
	host = strings.TrimPrefix(srv.URL, "http://")

	// override the package's moduleHTTPClient so the test doesn't need
	// any special transport - a vanilla client speaks plain HTTP to the
	// httptest server.
	prev := moduleHTTPClient
	moduleHTTPClient = &http.Client{}

	restore = func() {
		moduleHTTPClient = prev
		srv.Close()
	}
	return host, host, restore
}

// writeJSONResponse marshals data into the apiserver_lib.Response
// envelope and writes it to w. Mirrors what a real module API server
// returns for list and by-id GETs.
func writeJSONResponse(t *testing.T, w http.ResponseWriter, data []apiserver_lib.Object) {
	t.Helper()
	body, err := json.Marshal(apiserver_lib.Response{Data: data})
	require.NoError(t, err)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// TestGetNamesFromModule_HappyPath drives one GET per id and folds
// each module response's Name field into the returned map.
func TestGetNamesFromModule_HappyPath(t *testing.T) {
	var gotURLs []string
	endpoint, _, restore := fakeModuleServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotURLs = append(gotURLs, r.URL.String())
		switch r.URL.Path {
		case "/example.com/v0/widgets/1":
			writeJSONResponse(t, w, []apiserver_lib.Object{map[string]interface{}{"Name": "widget-one"}})
		case "/example.com/v0/widgets/2":
			writeJSONResponse(t, w, []apiserver_lib.Object{map[string]interface{}{"Name": "widget-two"}})
		default:
			http.NotFound(w, r)
		}
	})
	defer restore()

	out, err := getNamesFromModule(endpoint, "/example.com/v0/widgets", []uint{1, 2}, false)
	require.NoError(t, err)
	assert.Equal(t, map[uint]string{1: "widget-one", 2: "widget-two"}, out)
	assert.Len(t, gotURLs, 2, "exactly one GET per id")
}

// TestGetNamesFromModule_IncludeDeleted appends the
// IncludeDeleted query param when the caller opts in. Soft-delete
// gating is otherwise transparent.
func TestGetNamesFromModule_IncludeDeleted(t *testing.T) {
	var gotQuery string
	endpoint, _, restore := fakeModuleServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeJSONResponse(t, w, []apiserver_lib.Object{map[string]interface{}{"Name": "widget-one"}})
	})
	defer restore()

	_, err := getNamesFromModule(endpoint, "/example.com/v0/widgets", []uint{1}, true)
	require.NoError(t, err)
	assert.Equal(t, apiserver_lib.QueryParamIncludeDeleted+"=true", gotQuery)
}

// TestGetNamesFromModule_PartialFailure skips ids whose module
// lookups fail or return empty payloads, returning a map with just
// the successful entries rather than failing the whole batch.
func TestGetNamesFromModule_PartialFailure(t *testing.T) {
	endpoint, _, restore := fakeModuleServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/example.com/v0/widgets/1":
			writeJSONResponse(t, w, []apiserver_lib.Object{map[string]interface{}{"Name": "widget-one"}})
		case "/example.com/v0/widgets/2":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "/example.com/v0/widgets/3":
			writeJSONResponse(t, w, []apiserver_lib.Object{})
		default:
			http.NotFound(w, r)
		}
	})
	defer restore()

	out, err := getNamesFromModule(endpoint, "/example.com/v0/widgets", []uint{1, 2, 3}, false)
	require.NoError(t, err)
	assert.Equal(t, map[uint]string{1: "widget-one"}, out, "only successful lookups appear")
}

// TestGetNamesFromModule_DropsEmptyName drops ids whose response
// has a non-string or empty Name. The caller renders id-only when
// the name isn't present.
func TestGetNamesFromModule_DropsEmptyName(t *testing.T) {
	endpoint, _, restore := fakeModuleServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/example.com/v0/widgets/1":
			writeJSONResponse(t, w, []apiserver_lib.Object{map[string]interface{}{"Name": ""}})
		case "/example.com/v0/widgets/2":
			writeJSONResponse(t, w, []apiserver_lib.Object{map[string]interface{}{"NotName": "x"}})
		default:
			http.NotFound(w, r)
		}
	})
	defer restore()

	out, err := getNamesFromModule(endpoint, "/example.com/v0/widgets", []uint{1, 2}, false)
	require.NoError(t, err)
	assert.Empty(t, out)
}

// TestGetIDsFromModuleByName_HappyPath issues a single name-filtered
// GET and pulls out every row's ID. The name lookup is the inverse
// of the id lookup and is used by `tptctl events --for foo=name`.
func TestGetIDsFromModuleByName_HappyPath(t *testing.T) {
	var gotURL string
	endpoint, _, restore := fakeModuleServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		writeJSONResponse(t, w, []apiserver_lib.Object{
			map[string]interface{}{"ID": float64(7), "Name": "widget-seven"},
			map[string]interface{}{"ID": float64(9), "Name": "widget-seven"},
		})
	})
	defer restore()

	ids, err := getIDsFromModuleByName(endpoint, "/example.com/v0/widgets", "example.com/v0.widget", "widget-seven")
	require.NoError(t, err)
	assert.Equal(t, []uint{7, 9}, ids, "every matching row's id is collected")
	assert.Equal(t, "/example.com/v0/widgets?name=widget-seven", gotURL)
}

// TestGetIDsFromModuleByName_Empty handles the empty-result case as a
// successful zero-length slice, not an error - "no rows" is a
// legitimate answer for `name=missing`.
func TestGetIDsFromModuleByName_Empty(t *testing.T) {
	endpoint, _, restore := fakeModuleServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(t, w, []apiserver_lib.Object{})
	})
	defer restore()

	ids, err := getIDsFromModuleByName(endpoint, "/example.com/v0/widgets", "example.com/v0.widget", "ghost")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// TestGetIDsFromModuleByName_ModuleError surfaces a real transport
// or non-200 error to the caller with the object type in the message
// so the api-server log line points at the right type.
func TestGetIDsFromModuleByName_ModuleError(t *testing.T) {
	endpoint, _, restore := fakeModuleServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	defer restore()

	_, err := getIDsFromModuleByName(endpoint, "/example.com/v0/widgets", "example.com/v0.widget", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "example.com/v0.widget", "object type appears in error")
}

// TestParseRowID exercises the ID-field shape coverage directly. The
// httptest tests above exercise the float64 path through GetResponse
// (which uses json.Unmarshal without UseNumber). This test covers the
// json.Number branch the resolver defends against in case a caller's
// decoder ever swaps to UseNumber, plus the unrecognized-shape fallback.
func TestParseRowID(t *testing.T) {
	cases := []struct {
		name           string
		input          interface{}
		wantID         uint
		wantRecognized bool
		wantErr        bool
	}{
		{name: "float64 (default decoder shape)", input: float64(42), wantID: 42, wantRecognized: true},
		{name: "json.Number numeric", input: json.Number("7"), wantID: 7, wantRecognized: true},
		{name: "json.Number non-integer surfaces parse error", input: json.Number("not-a-number"), wantRecognized: true, wantErr: true},
		{name: "string is unrecognized, no error", input: "42"},
		{name: "nil is unrecognized, no error", input: nil},
		{name: "bool is unrecognized, no error", input: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, recognized, err := parseRowID(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, recognized, "json.Number stays recognized even on parse error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantRecognized, recognized)
			assert.Equal(t, tc.wantID, id)
		})
	}
}
