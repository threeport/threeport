package v0

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// sharedDbCredsMutex guards sharedDbCredsByNamespace.
var sharedDbCredsMutex sync.Mutex

// sharedDbCredsByNamespace holds the database credentials generated for each
// namespace. Generating a set takes four RSA 4096 bit keys, which is seconds
// per test, so tests that only need a valid set share one.
var sharedDbCredsByNamespace = map[string]*auth.DbCreds{}

// sharedDbCreds returns database credentials for a namespace, generating them
// the first time a test asks. Callers must not modify the result.
func sharedDbCreds(t *testing.T, namespace string) *auth.DbCreds {
	t.Helper()

	// reuse the set already generated for the namespace
	sharedDbCredsMutex.Lock()
	defer sharedDbCredsMutex.Unlock()
	if creds, ok := sharedDbCredsByNamespace[namespace]; ok {
		return creds
	}

	// generate and keep a set for the namespace
	creds, err := auth.GenerateDbCreds(namespace)
	require.NoError(t, err)
	sharedDbCredsByNamespace[namespace] = creds

	return creds
}
