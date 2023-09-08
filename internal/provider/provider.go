package provider

import (
	"fmt"

	kube "github.com/threeport/threeport/pkg/kube/v0"
)

const (
	// Max length of runtime names prevents infra provider resource names
	// exceeding maximum lengths imposed by provider.
	RuntimeNameMaxLength = 30
)

// KubernetesRuntimeInfra is the interface each provider has to satisfy to manage
// Kubernetes runtime infra.
type KubernetesRuntimeInfra interface {
	Create() (*kube.KubeConnectionInfo, error)
	Delete() error
}

// ThreeportRuntimeName returns the name for a Kubernetes runtime that hosts the
// threeport control plane.
func ThreeportRuntimeName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}
