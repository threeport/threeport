package provider

import (
	"fmt"

	"github.com/threeport/threeport/internal/kube"
)

// KubernetesRuntimeInfra is the interface each provider has to satisfy to manage
// Kubernetes runtime infra.
type KubernetesRuntimeInfra interface {
	Create() (*kube.KubeConnectionInfo, error)
	Delete() error
}

// ThreeportRuntimeName returns a name to use for a Kubernetes cluster where the
// threeport control plane runs.
func ThreeportRuntimeName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}
