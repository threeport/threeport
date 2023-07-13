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

// ClusterInfraProvider indicates which infrastructure provider is being
// used to run the threeport control plane.
type ClusterInfraProvider string

const (
	ClusterInfraProviderKind = "kind"
	ClusterInfraProviderEKS  = "eks"
)

// SupportedInfraProviders returns all supported infra providers.
// TODO: move this to code generated from constants above
func SupportedInfraProviders() []ClusterInfraProvider {
	return []ClusterInfraProvider{
		ClusterInfraProviderKind,
		ClusterInfraProviderEKS,
	}
}

// ControlPlaneTier denotes what level of availability and data retention is
// employed for an installation of a threeport control plane.
type ControlPlaneTier string

const (
	ControlPlaneTierDev  = "development"
	ControlPlaneTierProd = "production"
)

// ControlPlane is an instance of a threeport control plane.
type ControlPlane struct {
	InfraProvider ClusterInfraProvider
	Tier          ControlPlaneTier
}

// BootstrapKubernetesRuntimeName is the name given to the runtime cluster used
// as the initial compute space.
func BootstrapKubernetesRuntimeName(threeportInstanceName string) string {
	return fmt.Sprintf("compute-space-%s-0", threeportInstanceName)
}
