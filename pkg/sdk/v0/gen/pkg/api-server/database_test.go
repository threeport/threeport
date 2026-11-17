package apiserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// generateDatabaseInit writes the database package source into a scratch
// directory and returns it.
func generateDatabaseInit(t *testing.T, generator *gen.Generator) string {
	t.Helper()
	t.Chdir(t.TempDir())
	require.NoError(t, GenDatabaseInit(generator, moduleFixtureSdkConfig()))
	generated, err := os.ReadFile(filepath.Join("pkg", "api-server", "v0", "database", "database_gen.go"))
	require.NoError(t, err)

	return string(generated)
}

// TestAModuleConnectsWithItsUsersPassword checks the connection string of a
// module's API server and migrator: with the password of the module's own
// user, verifying the database with its CA's certificate and sending no
// client certificate, unless the root user is asked for.  A module installed
// before modules had users, which has no password, still connects with the
// client certificates.  The control plane connects as before.
func TestAModuleConnectsWithItsUsersPassword(t *testing.T) {
	module := generateDatabaseInit(t, moduleFixtureGenerator())
	assert.Contains(t, module, `if password, ok := os.LookupEnv("DB_PASSWORD"); ok && password != "" && !rootDbUser {`)
	assert.Contains(t, module, `"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s sslrootcert=%s/ca.crt TimeZone=UTC"`)
	assert.Contains(t, module, `sslcert=%[6]s/client.%[3]s.crt`, "a module without a password")

	core := generateDatabaseInit(t, &gen.Generator{})
	assert.NotContains(t, core, "DB_PASSWORD")
}
