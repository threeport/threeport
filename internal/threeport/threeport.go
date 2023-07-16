package threeport

import (
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

const (
	// The Kubernetes namespace in which the threeport control plane is
	// installed
	ControlPlaneNamespace = "threeport-control-plane"
)

// ControlPlaneTier denotes what level of availability and data retention is
// employed for an installation of a threeport control plane.
type ControlPlaneTier string

const (
	ControlPlaneTierDev  = "development"
	ControlPlaneTierProd = "production"
)

// ControlPlane is an instance of a threeport control plane.
type ControlPlane struct {
	InfraProvider v0.KubernetesRuntimeInfraProvider
	Tier          ControlPlaneTier
}

// BootstrapKubernetesRuntimeName is the name given to the runtime cluster used
// as the initial compute space.
func BootstrapKubernetesRuntimeName(threeportInstanceName string) string {
	return fmt.Sprintf("compute-space-%s-0", threeportInstanceName)
}
