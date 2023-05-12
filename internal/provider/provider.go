package provider

import (
	"fmt"

	"github.com/threeport/threeport/internal/kube"
)

// ControlPlaneInfra is the interface each provider has to satisfy to manage
// control plane infra.
type ControlPlaneInfra interface {
	Create(providerConfigDir string) (*kube.KubeConnectionInfo, error)
	Delete(providerConfigDir string) error
}

// ThreeportClusterName returns a name to use for a Kubernetes cluster where the
// threeport control plane runs.
func ThreeportClusterName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}
