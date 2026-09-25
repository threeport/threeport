package v0

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// emptyDataServer answers every request with 200 and no objects in Data, which
// is the shape the generated clients used to index into.
func emptyDataServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// a create is answered 201, everything else 200
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		w.WriteHeader(status)
		fmt.Fprint(w, `{"Data":[]}`)
	}))
}

// apiAddr returns the address in the form the generated clients expect: the
// response helper adds the scheme itself.
func apiAddr(server *httptest.Server) string {
	return strings.TrimPrefix(server.URL, "http://")
}

// TestClientsRejectEmptyResponseData covers the five operations that name a
// single object. The API answers 404 rather than 200 with nothing when the
// object is not there, so an empty Data means the server broke that contract -
// and reading the first element of it turned that into an index-out-of-range
// panic in the calling process, which for a controller is a crash loop.
//
// One object type stands in for all of them: the guard is emitted from one
// place in the SDK client template, so a type that has it covers the shape.
func TestClientsRejectEmptyResponseData(t *testing.T) {
	server := emptyDataServer(t)
	defer server.Close()

	client := server.Client()
	addr := apiAddr(server)
	secretDefinition := &v0.SecretDefinition{
		Definition: v0.Definition{Name: util.Ptr("test")},
		Common:     v0.Common{ID: util.Ptr(uint(1))},
	}

	operations := []struct {
		name string
		call func() error
	}{
		{
			name: "get by id",
			call: func() error {
				_, err := GetSecretDefinitionByID(client, addr, 1)

				return err
			},
		},
		{
			name: "create",
			call: func() error {
				_, err := CreateSecretDefinition(client, addr, secretDefinition)

				return err
			},
		},
		{
			name: "update",
			call: func() error {
				_, err := UpdateSecretDefinition(client, addr, secretDefinition)

				return err
			},
		},
		{
			name: "replace",
			call: func() error {
				_, err := ReplaceSecretDefinition(client, addr, secretDefinition)

				return err
			},
		},
		{
			name: "delete",
			call: func() error {
				_, err := DeleteSecretDefinition(client, addr, 1)

				return err
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() { err = operation.call() })
			require.Error(t, err, "an empty Data has to be reported, not indexed")
			assert.Contains(t, err.Error(), "no object in response data")
		})
	}
}
