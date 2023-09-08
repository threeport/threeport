package threeport

import (
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

const (
	// The Kubernetes namespace in which the threeport control plane is
	// installed
	ControlPlaneNamespace = "threeport-control-plane"

	// The maximum length of a threeport instance name is currently limited by
	// the length of role names in AWS which must include the threeport instance
	// name to preserve global uniqueness.
	// * AWS role name max length = 64 chars
	// * Allow 15 chars for role names (defined in eks-cluster)
	// * Allow 10 chars for "threeport-" prefix
	InstanceNameMaxLength = 30
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
