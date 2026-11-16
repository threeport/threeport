package kubernetesruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	tpi "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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

// TestSetComputeSpaceAgentImage checks which image a managed cluster pulls.
func TestSetComputeSpaceAgentImage(t *testing.T) {
	// the compiled image is what the installer starts with
	const (
		compiledNamespace = "ghcr.io/threeport"
		compiledTag       = "v0.7.0-rc.2"
		recordedNamespace = "ghcr.io/threeport"
		recordedName      = "threeport-agent"
		recordedTag       = "v1.2.3"
		apiNamespace      = "us-central1-docker.pkg.dev/example/threeport"
		apiTag            = "abcdef12-0123456789ab"
	)

	compiled := func() *v0.ControlPlaneComponent {
		return &v0.ControlPlaneComponent{
			ImageNamespace: compiledNamespace,
			ImageTag:       compiledTag,
		}
	}
	plane := &v0.ControlPlaneInstance{
		ImageNamespace: util.Ptr(apiNamespace),
		ImageTag:       util.Ptr(apiTag),
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{
				Name:           "rest-api",
				ImageNamespace: apiNamespace,
				ImageTag:       apiTag,
			},
			{
				Name:           tpi.ThreeportAgentName,
				ImageNamespace: recordedNamespace,
				ImageName:      recordedName,
				ImageTag:       recordedTag,
			},
		},
	}

	t.Run("the recorded agent image", func(t *testing.T) {
		agent := compiled()
		require.NoError(t, setComputeSpaceAgentImage(agent, nil, plane))
		assert.Equal(t, recordedNamespace, agent.ImageNamespace)
		assert.Equal(t, recordedName, agent.ImageName)
		assert.Equal(t, recordedTag, agent.ImageTag)
	})

	t.Run("a runtime image wins", func(t *testing.T) {
		agent := compiled()
		agent.ImageName = "compiled-agent"
		image := "example.com/other/threeport-agent:custom"
		require.NoError(t, setComputeSpaceAgentImage(agent, &image, plane))
		assert.Equal(t, "example.com/other", agent.ImageNamespace)
		assert.Equal(t, "threeport-agent", agent.ImageName)
		assert.Equal(t, "custom", agent.ImageTag)
	})

	t.Run("an empty runtime image uses the recorded agent", func(t *testing.T) {
		agent := compiled()
		image := ""
		require.NoError(t, setComputeSpaceAgentImage(agent, &image, plane))
		assert.Equal(t, recordedNamespace, agent.ImageNamespace)
		assert.Equal(t, recordedTag, agent.ImageTag)
	})

	t.Run("a nil control plane leaves the compiled image", func(t *testing.T) {
		agent := compiled()
		require.NoError(t, setComputeSpaceAgentImage(agent, nil, nil))
		assert.Equal(t, compiledNamespace, agent.ImageNamespace)
		assert.Equal(t, compiledTag, agent.ImageTag)
	})

	t.Run("a namespace without a tag keeps the compiled tag", func(t *testing.T) {
		agent := compiled()
		plane := &v0.ControlPlaneInstance{
			ImageNamespace: util.Ptr(apiNamespace),
			ImageTag:       util.Ptr(apiTag),
			CustomComponentInfo: []*v0.ControlPlaneComponent{{
				Name:           tpi.ThreeportAgentName,
				ImageNamespace: recordedNamespace,
			}},
		}
		require.NoError(t, setComputeSpaceAgentImage(agent, nil, plane))
		assert.Equal(t, recordedNamespace, agent.ImageNamespace)
		assert.Equal(t, compiledTag, agent.ImageTag)
	})

	t.Run("instance fields without a recorded agent leave the compiled image", func(t *testing.T) {
		agent := compiled()
		plane := &v0.ControlPlaneInstance{
			ImageNamespace: util.Ptr(apiNamespace),
			ImageTag:       util.Ptr(apiTag),
		}
		require.NoError(t, setComputeSpaceAgentImage(agent, nil, plane))
		assert.Equal(t, compiledNamespace, agent.ImageNamespace)
		assert.Equal(t, compiledTag, agent.ImageTag)
	})

	t.Run("an image with no registry is an error", func(t *testing.T) {
		agent := compiled()
		image := "threeport-agent:custom"
		err := setComputeSpaceAgentImage(agent, &image, plane)
		assert.Error(t, err)
		assert.Equal(t, compiledNamespace, agent.ImageNamespace)
		assert.Equal(t, compiledTag, agent.ImageTag)
	})
}
