package kubernetesruntime

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	tpi "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// apiAuthEnabled reports whether this controller reaches the API with client
// certificate authentication, which is whether the control plane has auth on:
// the controller's client is built from the same setting the API is started
// with.  An agent on a managed cluster has to be installed to match.
func apiAuthEnabled(apiClient *http.Client) bool {
	if apiClient == nil {
		return false
	}
	transport, ok := apiClient.Transport.(*client_lib.CustomTransport)

	return ok && transport.IsTlsEnabled
}

// agentCertificateIssuer returns the call that gets a client certificate for
// the agent on the named runtime instance from the control plane's API.
//
// The agent on a managed cluster is given a certificate by the API rather than
// signed for here, because the CA's private key stays in the API's pod: this
// controller holds cloud credentials and cluster-admin on every managed
// cluster, and with the key it could also sign a certificate that any control
// plane component would accept.  It sends only the agent's public key, and the
// API decides the name the certificate carries.
func agentCertificateIssuer(r *controller.Reconciler, runtimeInstanceName string) tpi.AgentCertificateIssuer {
	return func(certificateSigningRequest string) (*v0.AgentCertificate, error) {
		issued, err := client.CreateAgentCertificate(
			r.APIClient,
			r.APIServer,
			runtimeInstanceName,
			certificateSigningRequest,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get an agent certificate for runtime instance %s: %w",
				runtimeInstanceName, err,
			)
		}

		return issued, nil
	}
}

// setComputeSpaceAgentImage sets the agent image a managed cluster pulls.
// A named runtime image wins. Otherwise it copies the agent image recorded on the control plane.
func setComputeSpaceAgentImage(
	agent *v0.ControlPlaneComponent,
	runtimeImage *string,
	controlPlane *v0.ControlPlaneInstance,
) error {
	// use the image the runtime names
	if image := util.DerefString(runtimeImage); image != "" {
		registry, name, tag, err := util.ParseImage(image)
		if err != nil {
			return fmt.Errorf("failed to parse custom threeport agent image: %w", err)
		}
		agent.ImageNamespace = registry
		agent.ImageName = name
		agent.ImageTag = tag
		return nil
	}

	// copy the agent image the control plane recorded
	var recorded *v0.ControlPlaneComponent
	if controlPlane != nil {
		for _, component := range controlPlane.CustomComponentInfo {
			if component != nil && component.Name == tpi.ThreeportAgentName {
				recorded = component
				break
			}
		}
	}
	if recorded == nil {
		return nil
	}
	if recorded.ImageNamespace != "" {
		agent.ImageNamespace = recorded.ImageNamespace
	}
	if recorded.ImageName != "" {
		agent.ImageName = recorded.ImageName
	}
	if recorded.ImageTag != "" {
		agent.ImageTag = recorded.ImageTag
	}

	return nil
}
