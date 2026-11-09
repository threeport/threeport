package v0

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeneratedReconcilerEnsuresSupportServices checks the generated reconciler
// calls each support-service ensure.
func TestGeneratedReconcilerEnsuresSupportServices(t *testing.T) {
	// walk up from this file to the generated instance reconciler
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	var reconciler string
	for {
		candidate := filepath.Join(dir, "internal", "widget", "widget_instance_reconciler_gen.go")
		if _, err := os.Stat(candidate); err == nil {
			reconciler = candidate
			break
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir)
		dir = parent
	}
	body, err := os.ReadFile(reconciler)
	require.NoError(t, err)
	text := string(body)

	// the four ensure calls are present
	require.Contains(t, text, "EnsureIngressComponent(")
	require.Contains(t, text, "EnsureCertificatesComponent(")
	require.Contains(t, text, "EnsureSecretsComponent(")
	require.Contains(t, text, "EnsureDatabaseComponent(")
}
