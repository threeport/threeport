package v0

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestGetTerraformInstanceShowsItsOutputs covers #569: the outputs of a
// terraform instance - an RDS database's host and password, say - are what an
// operator reads it for, and `tptctl describe` that used to show them is gone.
// `tptctl get terraform-instance` shows them instead, redacted unless the
// secrets are asked to be decrypted, like every other encrypted field.
func TestGetTerraformInstanceShowsItsOutputs(t *testing.T) {
	const outputs = `{"rds_hostname":{"sensitive":true,"type":"string","value":"wordpress-db.example.us-east-2.rds.amazonaws.com"}}`

	encryptionKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	encrypted, err := encryption.Encrypt(encryptionKey, outputs)
	require.NoError(t, err)

	name := "rds-instance-01"
	createdAt := time.Now()
	stored := api_v0.TerraformInstance{
		Common:         api_v0.Common{ID: util.Ptr(uint(1)), CreatedAt: &createdAt},
		Instance:       api_v0.Instance{Name: &name},
		Reconciliation: api_v0.Reconciliation{Reconciled: util.Ptr(true), CreationFailed: util.Ptr(false)},
		Outputs:        &encrypted,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWithObject(t, w, stored)
	}))
	defer srv.Close()

	get := func(key string) string {
		config := TerraformInstanceConfig{TerraformInstance: TerraformInstanceValues{Name: &name}}
		instances, err := config.Get(&http.Client{}, strings.TrimPrefix(srv.URL, "http://"), key)
		require.NoError(t, err)
		require.Len(t, *instances, 1)
		require.NotNil(t, (*instances)[0].TerraformInstance.Outputs, "the outputs are part of what get returns")

		return *(*instances)[0].TerraformInstance.Outputs
	}

	assert.Equal(t, encryption.RedactedValuePlaceholder, get(""), "redacted without --decrypt-secrets")
	assert.Equal(t, outputs, get(encryptionKey), "readable with --decrypt-secrets")
}
