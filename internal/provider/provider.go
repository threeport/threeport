package provider

import (
	"fmt"

	"github.com/threeport/threeport/internal/kube"
)

//// ControlPlane contains the attributes of a threeport control plane.
//type ControlPlaneInfraProvider struct {
//	InstanceName           string
//	ProviderAccountID      string
//	MinClusterNodes        int32
//	MaxClusterNodes        int32
//	DefaultAWSInstanceType string
//	RootDomainName         string
//	AdminEmail             string
//}

type ControlPlaneInfra interface {
	Create() (*kube.KubeConnectionInfo, error)
	Delete() error
}

// ThreeportClusterName returns a name to use for a Kubernetes cluster where the
// threeport control plane runs.
func ThreeportClusterName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}
