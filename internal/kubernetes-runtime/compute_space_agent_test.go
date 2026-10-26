package kubernetesruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

func TestLoadComputeSpaceAgentAccess(t *testing.T) {
	const endpoint = "34.1.2.3:443"

	t.Run("none when the control plane has no external endpoint", func(t *testing.T) {
		access, err := loadComputeSpaceAgentAccess("", t.TempDir())
		require.NoError(t, err)
		assert.Nil(t, access)
		assert.Nil(t, agentAuthConfig(access))
	})

	t.Run("an address without a signing CA is a control plane with auth off", func(t *testing.T) {
		access, err := loadComputeSpaceAgentAccess(endpoint, filepath.Join(t.TempDir(), "absent"))
		require.NoError(t, err)
		require.NotNil(t, access)
		assert.Equal(t, endpoint, access.apiServer)
		assert.Nil(t, access.authConfig)
	})

	t.Run("the signing CA is loaded to issue certificates from", func(t *testing.T) {
		generated, err := auth.GetAuthConfig()
		require.NoError(t, err)
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), []byte(generated.CAPemEncoded), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.key"), []byte(generated.CAPrivateKeyPemEncoded), 0o600))

		access, err := loadComputeSpaceAgentAccess(endpoint, dir)
		require.NoError(t, err)
		require.NotNil(t, access.authConfig)
		assert.Equal(t, generated.CAPemEncoded, access.authConfig.CAPemEncoded)
		assert.Same(t, access.authConfig, agentAuthConfig(access))
	})

	// half a keypair is a broken mount, not a control plane without auth
	t.Run("a certificate without its key is an error", func(t *testing.T) {
		generated, err := auth.GetAuthConfig()
		require.NoError(t, err)
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), []byte(generated.CAPemEncoded), 0o600))

		_, err = loadComputeSpaceAgentAccess(endpoint, dir)
		assert.Error(t, err)
	})

	t.Run("a key that does not match the certificate is an error", func(t *testing.T) {
		first, err := auth.GetAuthConfig()
		require.NoError(t, err)
		second, err := auth.GetAuthConfig()
		require.NoError(t, err)
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), []byte(first.CAPemEncoded), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.key"), []byte(second.CAPrivateKeyPemEncoded), 0o600))

		_, err = loadComputeSpaceAgentAccess(endpoint, dir)
		assert.Error(t, err)
	})
}
