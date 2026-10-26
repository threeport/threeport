package kubernetesruntime

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// computeSpaceAgentAccess is what the agent on a managed cluster needs to report
// to the control plane that manages it.
type computeSpaceAgentAccess struct {
	// apiServer is the address, as host:port, the control plane's API is
	// reachable at from the managed cluster.
	apiServer string

	// authConfig is the control plane's CA, to issue the agent's client
	// certificate from.  Nil when the control plane runs without auth.
	authConfig *auth.AuthConfig
}

// loadComputeSpaceAgentAccess reads what an agent on a managed cluster needs.
//
// A managed cluster does not host the control plane, so its agent cannot use the
// API's in-cluster service: it is given the control plane's external address,
// and when the control plane has auth enabled a client certificate issued from
// its CA.  The address is the controller's own configuration; the CA is the
// signing keypair mounted into this controller at signingCaDir.
//
// It returns nil when the control plane has no external address.  Such a
// control plane cannot be reached from another cluster, so an agent installed
// there could only crash-loop.
func loadComputeSpaceAgentAccess(
	apiServer string,
	signingCaDir string,
) (*computeSpaceAgentAccess, error) {
	if apiServer == "" {
		return nil, nil
	}

	certPath := filepath.Join(signingCaDir, "tls.crt")
	keyPath := filepath.Join(signingCaDir, "tls.key")
	caPem, certErr := os.ReadFile(certPath)
	caKeyPem, keyErr := os.ReadFile(keyPath)

	// neither file present means the control plane has auth off and the
	// controller was not given the signing CA
	if errors.Is(certErr, fs.ErrNotExist) && errors.Is(keyErr, fs.ErrNotExist) {
		return &computeSpaceAgentAccess{apiServer: apiServer}, nil
	}
	if certErr != nil {
		return nil, fmt.Errorf("failed to read the signing CA certificate: %w", certErr)
	}
	if keyErr != nil {
		return nil, fmt.Errorf("failed to read the signing CA private key: %w", keyErr)
	}

	authConfig, err := auth.LoadAuthConfig(caPem, caKeyPem)
	if err != nil {
		return nil, fmt.Errorf("failed to load the signing CA: %w", err)
	}

	return &computeSpaceAgentAccess{apiServer: apiServer, authConfig: authConfig}, nil
}

// agentAuthConfig returns the CA to issue an agent certificate from, or nil when
// there is none to issue from.
func agentAuthConfig(access *computeSpaceAgentAccess) *auth.AuthConfig {
	if access == nil {
		return nil
	}

	return access.authConfig
}
