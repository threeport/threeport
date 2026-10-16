package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// controlPlaneDefinitionReplaceServer stands in for the API during a Replace.
//
// Replace looks the definition up by name and then PUTs it back, so the
// server answers the GET with the stored object and records whatever PUT
// body arrives - the payload is what the tests are about.
func controlPlaneDefinitionReplaceServer(
	t *testing.T,
	stored api_v0.ControlPlaneDefinition,
) (string, *http.Client, *[]api_v0.ControlPlaneDefinition) {
	t.Helper()
	var puts []api_v0.ControlPlaneDefinition

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
				Data:   []apiserver_lib.Object{stored},
			})
			if !assert.NoError(t, err) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)

		case http.MethodPut:
			var received api_v0.ControlPlaneDefinition
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&received)) {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			puts = append(puts, received)

			body, err := json.Marshal(apiserver_lib.Response{
				Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
				Data:   []apiserver_lib.Object{received},
			})
			if !assert.NoError(t, err) {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)

		default:
			http.Error(w, "unexpected method "+r.Method, http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)

	// the client builds the URL as "http://" + endpoint, so the scheme the
	// test server reports has to come off before it is handed over
	return strings.TrimPrefix(srv.URL, "http://"), srv.Client(), &puts
}

// storedControlPlaneDefinition is a definition as the API holds it.
func storedControlPlaneDefinition(name, tier string) api_v0.ControlPlaneDefinition {
	createdAt := time.Now()

	return api_v0.ControlPlaneDefinition{
		Common:      api_v0.Common{ID: util.Ptr(uint(1)), CreatedAt: &createdAt},
		Definition:  api_v0.Definition{Name: util.Ptr(name)},
		AuthEnabled: util.Ptr(false),
		Tier:        util.Ptr(tier),
	}
}

// TestControlPlaneDefinitionConfig_Replace_TierIsImmutable covers the tier at
// the layer the user actually edits.
//
// The API refuses a tier change as well, but a config file is applied whole:
// the caller edits one field and the rest is sent back as it was read. Left
// to the API, changing the tier in a config would come back as a 400 naming a
// request the caller never wrote. Refusing it here names the config instead.
func TestControlPlaneDefinitionConfig_Replace_TierIsImmutable(t *testing.T) {
	t.Run("a config naming a different tier is refused", func(t *testing.T) {
		stored := storedControlPlaneDefinition("cpd", api_v0.ControlPlaneTierDevelopment)
		endpoint, client, puts := controlPlaneDefinitionReplaceServer(t, stored)

		config := &ControlPlaneDefinitionConfig{
			ControlPlaneDefinition: ControlPlaneDefinitionValues{
				Name: util.Ptr("cpd"),
				Tier: util.Ptr(api_v0.ControlPlaneTierProduction),
			},
		}
		_, err := config.Replace(client, endpoint, "cpd")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be changed")
		assert.Contains(t, err.Error(), api_v0.ControlPlaneTierDevelopment, "the error names what is recorded")
		assert.Contains(t, err.Error(), api_v0.ControlPlaneTierProduction, "and what was asked for")
		assert.Empty(t, *puts, "a refused change must not reach the API")
	})

	t.Run("a config that omits the tier carries the recorded one forward", func(t *testing.T) {
		stored := storedControlPlaneDefinition("cpd", api_v0.ControlPlaneTierProduction)
		endpoint, client, puts := controlPlaneDefinitionReplaceServer(t, stored)

		config := &ControlPlaneDefinitionConfig{
			ControlPlaneDefinition: ControlPlaneDefinitionValues{
				Name:        util.Ptr("cpd"),
				AuthEnabled: util.Ptr(true),
			},
		}
		_, err := config.Replace(client, endpoint, "cpd")
		require.NoError(t, err)
		require.Len(t, *puts, 1)
		require.NotNil(t, (*puts)[0].Tier, "a replace must not clear the tier")
		assert.Equal(t, api_v0.ControlPlaneTierProduction, *(*puts)[0].Tier)
	})

	t.Run("a config restating the recorded tier is accepted", func(t *testing.T) {
		stored := storedControlPlaneDefinition("cpd", api_v0.ControlPlaneTierProduction)
		endpoint, client, puts := controlPlaneDefinitionReplaceServer(t, stored)

		config := &ControlPlaneDefinitionConfig{
			ControlPlaneDefinition: ControlPlaneDefinitionValues{
				Name: util.Ptr("cpd"),
				Tier: util.Ptr(api_v0.ControlPlaneTierProduction),
			},
		}
		_, err := config.Replace(client, endpoint, "cpd")
		require.NoError(t, err)
		require.Len(t, *puts, 1, "restating the value it already holds is not a change")
		require.NotNil(t, (*puts)[0].Tier)
		assert.Equal(t, api_v0.ControlPlaneTierProduction, *(*puts)[0].Tier)
	})
}
