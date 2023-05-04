package threeport

import "fmt"

const (
	// The Kubernetes namespace in which the threeport control plane is
	// installed
	ControlPlaneNamespace = "threeport-control-plane"
)

// ControlPlaneInfraProvider indicates which infrastructure provider is being
// used to run the threeport control plane.
type ControlPlaneInfraProvider string

const (
	ControlPlaneInfraProviderKind = "kind"
	ControlPlaneInfraProviderEKS  = "eks"
)

// SupportedInfraProviders returns all supported infra providers.
// TODO: move this to code generated from constants above
func SupportedInfraProviders() []ControlPlaneInfraProvider {
	return []ControlPlaneInfraProvider{
		ControlPlaneInfraProviderKind,
		ControlPlaneInfraProviderEKS,
	}
}

// ControlPlaneTier denotes what level of availability and data retention is
// employed for an installation of a threeport control plane.
type ControlPlaneTier string

const (
	ControlPlaneTierDev  = "development"
	ControlPlaneTierProd = "production"
)

// ControlPlane is an instance of a threeport control plane
type ControlPlane struct {
	InfraProvider ControlPlaneInfraProvider
	Tier          ControlPlaneTier
}

func BootstrapClusterName(threeportInstanceName string) string {
	return fmt.Sprintf("compute-space-%s-0", threeportInstanceName)
}
