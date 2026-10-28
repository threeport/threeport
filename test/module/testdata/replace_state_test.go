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

// TestInstanceReplace_KeepsTheReconcilersState covers the Replace the SDK
// scaffolds, before a module author adds anything to it. Replace sends a PUT,
// which writes every field, so a payload that left out the reconciler's
// fields would clear them on every replace. The scaffold starts from a copy
// of the object it fetched, so they go back as they were.
func TestInstanceReplace_KeepsTheReconcilersState(t *testing.T) {
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPut {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"Data":[{"ID":9,"Name":"renamed","CreatedAt":"2026-09-01T00:00:00Z"}]}`)

			return
		}

		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "widget-definitions") {
			fmt.Fprint(w, `{"Data":[{"ID":7,"Name":"the-definition","CreatedAt":"2026-09-01T00:00:00Z"}]}`)

			return
		}
		fmt.Fprint(w, `{"Data":[{
			"ID":9,
			"Name":"w",
			"Status":"Running",
			"Reconciled":true,
			"CreationConfirmed":"2026-09-01T00:05:00Z",
			"WidgetDefinitionID":7,
			"CreatedAt":"2026-09-01T00:00:00Z"
		}]}`)
	}))
	defer server.Close()

	config := WidgetInstanceConfig{WidgetInstance: WidgetInstanceValues{
		Name:             util.Ptr("renamed"),
		WidgetDefinition: &WidgetDefinitionValues{Name: util.Ptr("the-definition")},
	}}

	_, err := config.Replace(server.Client(), apiAddr(server), "w")
	require.NoError(t, err)

	require.NotNil(t, sent, "the replace sent no PUT")
	assert.Equal(t, "renamed", sent["Name"], "the config's name is what the replace sets")
	assert.Equal(t, "Running", sent["Status"])
	assert.Equal(t, true, sent["Reconciled"])
	assert.Equal(t, "2026-09-01T00:05:00Z", sent["CreationConfirmed"], "every reconciliation field is kept, not only Reconciled")
	assert.EqualValues(t, 7, sent["WidgetDefinitionID"])
	for _, field := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt"} {
		assert.NotContains(t, sent, field, "the API refuses a replace carrying %s", field)
	}
}
