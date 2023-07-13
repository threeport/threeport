package provider

import (
	"fmt"

	"github.com/threeport/threeport/internal/kube"
)

// ClusterInfra is the interface each provider has to satisfy to manage
// Kubernetes runtime infra.
type ClusterInfra interface {
	Create() (*kube.KubeConnectionInfo, error)
	Delete() error
}

// ThreeportClusterName returns a name to use for a Kubernetes cluster where the
// threeport control plane runs.
func ThreeportClusterName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}
