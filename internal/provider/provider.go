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

// ThreeportRuntimeName returns the name for a Kubernetes runtime that hosts the
// threeport control plane.
func ThreeportRuntimeName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}
