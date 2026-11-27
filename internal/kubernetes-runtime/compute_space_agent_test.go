package kubernetesruntime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	tpi "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAPIAuthEnabled covers how the controller tells whether the control plane
// has authentication on: its client is built with client certificates exactly
// when it does.
func TestAPIAuthEnabled(t *testing.T) {
	withAuth := &http.Client{Transport: &client_lib.CustomTransport{IsTlsEnabled: true}}
	withoutAuth := &http.Client{Transport: &client_lib.CustomTransport{IsTlsEnabled: false}}

	assert.True(t, apiAuthEnabled(withAuth))
	assert.False(t, apiAuthEnabled(withoutAuth))

	// no client, or one the controller did not build, is not one with auth
	assert.False(t, apiAuthEnabled(nil))
	assert.False(t, apiAuthEnabled(&http.Client{}))
}

// TestAgentCertificateIssuer covers the call the installer makes to get an
// agent's certificate: it goes to the API, for the right runtime, and says
// which runtime when it fails.
func TestAgentCertificateIssuer(t *testing.T) {
	t.Run("asks the API for the runtime's agent", func(t *testing.T) {
		var sent map[string]string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, v0.PathAgentCertificates, r.URL.Path)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"Data":[{"Certificate":"CERT","CACertificate":"CA"}]}`)
		}))
		defer server.Close()
		reconciler := &controller.Reconciler{
			APIClient: server.Client(),
			APIServer: strings.TrimPrefix(server.URL, "http://"),
		}

		issued, err := agentCertificateIssuer(reconciler, "east")("CSR")

		require.NoError(t, err)
		assert.Equal(t, "CERT", issued.Certificate)
		assert.Equal(t, "CA", issued.CACertificate)
		assert.Equal(t, map[string]string{"RuntimeInstanceName": "east", "CertificateSigningRequest": "CSR"}, sent)
	})

	t.Run("names the runtime when the API refuses", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"Status":{"Code":403,"Error":"refused"}}`)
		}))
		defer server.Close()
		reconciler := &controller.Reconciler{
			APIClient: server.Client(),
			APIServer: strings.TrimPrefix(server.URL, "http://"),
		}

		issued, err := agentCertificateIssuer(reconciler, "east")("CSR")

		require.Error(t, err)
		assert.Nil(t, issued)
		assert.Contains(t, err.Error(), "runtime instance east")
		assert.ErrorIs(t, err, client_lib.ErrForbidden)
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
