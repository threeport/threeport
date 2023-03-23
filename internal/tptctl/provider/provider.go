package provider

import "fmt"

// ControlPlane contains the attributes of a threeport control plane.
type ControlPlane struct {
	InstanceName           string
	ProviderAccountID      string
	MinClusterNodes        int32
	MaxClusterNodes        int32
	DefaultAWSInstanceType string
	RootDomainName         string
	AdminEmail             string
}

// NewControlPlane returns a ControlPlane with default values set.
func NewControlPlane() *ControlPlane {
	return &ControlPlane{
		InstanceName:           "threeport-control-plane",
		MinClusterNodes:        0,
		MaxClusterNodes:        4,
		DefaultAWSInstanceType: "t3.medium",
	}
}

// ThreeportClusterName returns a name to use for a Kubernetes cluster where the
// threeport control plane runs.
func (c *ControlPlane) ThreeportClusterName() string {
	return fmt.Sprintf("threeport-%s", c.InstanceName)
}
