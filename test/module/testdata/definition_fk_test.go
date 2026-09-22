package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// This file is tracked at test/module/testdata and copied into the generated
// config package before the module is built.

// definitionApi answers the two calls a create makes: the lookup that resolves
// the definition by name, and the create itself. It records the definition ID
// the created instance carried.
func definitionApi(t *testing.T, sentDefinitionId *uint) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if id, ok := body["WidgetDefinitionID"].(float64); ok {
				*sentDefinitionId = uint(id)
			}
			status := http.StatusCreated
			if r.Method == http.MethodPut {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			fmt.Fprint(w, `{"Data":[{"ID":9,"Name":"w","CreatedAt":"2026-09-01T00:00:00Z"}]}`)

			return
		}

		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "widget-definitions") {
			fmt.Fprint(w, `{"Data":[{"ID":7,"Name":"w","CreatedAt":"2026-09-01T00:00:00Z"}]}`)

			return
		}
		fmt.Fprint(w, `{"Data":[{"ID":9,"Name":"w","CreatedAt":"2026-09-01T00:00:00Z"}]}`)
	}))
}

// apiAddr returns the address in the form the generated clients expect: the
// response helper adds the scheme itself.
func apiAddr(server *httptest.Server) string {
	return strings.TrimPrefix(server.URL, "http://")
}

// TestInstanceCreate_SetsTheDefinitionForeignKey covers the key the API
// requires on an instance. Without it every create failed validation, and the
// combined flow left the definition it had already created behind.
func TestInstanceCreate_SetsTheDefinitionForeignKey(t *testing.T) {
	var sent uint
	server := definitionApi(t, &sent)
	defer server.Close()

	config := WidgetInstanceConfig{WidgetInstance: WidgetInstanceValues{
		Name:             util.Ptr("w"),
		WidgetDefinition: &WidgetDefinitionValues{Name: util.Ptr("w")},
	}}

	_, err := config.Create(server.Client(), apiAddr(server))
	require.NoError(t, err)
	assert.Equal(t, uint(7), sent, "the definition resolved by name has to reach the API object")
}

// TestInstanceCreate_PrefersAResolvedId covers the caller that already knows
// the ID - the combined flow creates the definition first - so the instance
// does not read back what was just written.
func TestInstanceCreate_PrefersAResolvedId(t *testing.T) {
	var sent uint
	server := definitionApi(t, &sent)
	defer server.Close()

	config := WidgetInstanceConfig{WidgetInstance: WidgetInstanceValues{
		Name: util.Ptr("w"),
		WidgetDefinition: &WidgetDefinitionValues{
			Name: util.Ptr("w"),
			ID:   util.Ptr(uint(42)),
		},
	}}

	_, err := config.Create(server.Client(), apiAddr(server))
	require.NoError(t, err)
	assert.Equal(t, uint(42), sent, "an ID in hand is used rather than looked up again")
}

// TestInstanceCreate_ReportsAMissingDefinition covers a config that names no
// definition at all, which the API would reject with a constraint error.
func TestInstanceCreate_ReportsAMissingDefinition(t *testing.T) {
	var sent uint
	server := definitionApi(t, &sent)
	defer server.Close()

	config := WidgetInstanceConfig{WidgetInstance: WidgetInstanceValues{Name: util.Ptr("w")}}

	_, err := config.Create(server.Client(), apiAddr(server))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WidgetDefinition")
	assert.Zero(t, sent, "nothing should have been written")
}

// TestInstanceReplace_SetsTheDefinitionForeignKey covers the same gap on the
// replace path, which is a full PUT and so has to carry the key too.
func TestInstanceReplace_SetsTheDefinitionForeignKey(t *testing.T) {
	var sent uint
	server := definitionApi(t, &sent)
	defer server.Close()

	config := WidgetInstanceConfig{WidgetInstance: WidgetInstanceValues{
		Name:             util.Ptr("w"),
		WidgetDefinition: &WidgetDefinitionValues{Name: util.Ptr("w")},
	}}

	_, err := config.Replace(server.Client(), apiAddr(server), "w")
	require.NoError(t, err)
	assert.Equal(t, uint(7), sent)
}

// TestInstanceGet_PopulatesTheDefinitionReference closes the loop the other
// two halves open: Create writes the key and Get reads it back as the name the
// config abstraction speaks in. Without it the reference is always nil and the
// pairing in mapTo...DefinedInstances has nothing to check.
func TestInstanceGet_PopulatesTheDefinitionReference(t *testing.T) {
	var definitionLookups int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if strings.Contains(r.URL.Path, "widget-definitions") {
			definitionLookups++
			fmt.Fprint(w, `{"Data":[{"ID":7,"Name":"the-definition","CreatedAt":"2026-09-01T00:00:00Z"}]}`)

			return
		}
		// two instances of one definition
		fmt.Fprint(w, `{"Data":[
			{"ID":1,"Name":"a","WidgetDefinitionID":7,"CreatedAt":"2026-09-01T00:00:00Z"},
			{"ID":2,"Name":"b","WidgetDefinitionID":7,"CreatedAt":"2026-09-01T00:00:00Z"}
		]}`)
	}))
	defer server.Close()

	config := WidgetInstanceConfig{}
	configs, err := config.Get(server.Client(), apiAddr(server))
	require.NoError(t, err)
	require.Len(t, *configs, 2)

	for _, returned := range *configs {
		require.NotNil(t, returned.WidgetInstance.WidgetDefinition, "the reference has to be populated")
		assert.Equal(t, "the-definition", *returned.WidgetInstance.WidgetDefinition.Name)
		assert.Equal(t, uint(7), *returned.WidgetInstance.WidgetDefinition.ID)
	}

	assert.Equal(
		t, 1, definitionLookups,
		"instances sharing a definition must cost one lookup, not one each",
	)
}
