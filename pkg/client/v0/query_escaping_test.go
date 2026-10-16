package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestALookupByNameAsksForTheNameItWasGiven covers the gap between the name a
// caller holds and the name the server is asked about.
//
// A name goes into a query string. Unescaped, "prod+ha" reaches the server as
// "prod ha" and the lookup misses the row it was asked for - and a miss is
// not an error, it is "no such object". Anything that looks something up to
// decide whether to create it then creates a second one, or fails on the
// unique name it is about to collide with.
//
// The characters below are the ones that change meaning in a query: + is a
// space, & starts another parameter, # starts a fragment and truncates the
// rest.
func TestALookupByNameAsksForTheNameItWasGiven(t *testing.T) {
	for _, name := range []string{
		"plain-name",
		"prod+ha",
		"a b",
		"a&b=c",
		"a#b",
		"100%",
		"café",
	} {
		t.Run(name, func(t *testing.T) {
			var asked string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				asked = r.URL.Query().Get("name")

				body, err := json.Marshal(apiserver_lib.Response{
					Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
					Data: []apiserver_lib.Object{api_v0.ControlPlaneDefinition{
						Definition: api_v0.Definition{Name: util.Ptr(asked)},
					}},
				})
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))
			defer server.Close()

			// the client builds the URL as "http://" + endpoint
			_, err := GetControlPlaneDefinitionByName(
				server.Client(), strings.TrimPrefix(server.URL, "http://"), name,
			)
			require.NoError(t, err)
			assert.Equal(
				t, name, asked,
				"the server was asked about a different name than the caller holds",
			)
		})
	}
}
