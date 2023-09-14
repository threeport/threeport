package v0

import (
	"github.com/threeport/threeport/internal/version"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

const (
	ThreeportImageRepo                        = "ghcr.io/threeport"
	ThreeportAPIImage                         = "threeport-rest-api"
	ThreeportWorkloadControllerImage          = "threeport-workload-controller"
	ThreeportKubernetesRuntimeControllerImage = "threeport-kubernetes-runtime-controller"
	ThreeportAwsControllerImage               = "threeport-aws-controller"
	ThreeportGatewayControllerImage           = "threeport-gateway-controller"
	ThreeportAgentDeployName                  = "threeport-agent"
	ThreeportAgentImage                       = "threeport-agent"
	ThreeportAPIServiceResourceName           = "threeport-api-server"
	ThreeportLocalAPIEndpoint                 = "localhost"
)

var ThreeportControllerList []*InstallInfo = []*InstallInfo{
	{
		Name:               "workload-controller",
		ImageName:          ThreeportWorkloadControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
	},
	{
		Name:               "kubernetes-runtime-controller",
		ImageName:          ThreeportKubernetesRuntimeControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
	},
	{
		Name:               "aws-controller",
		ImageName:          ThreeportAwsControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
	},
	{
		Name:               "gateway-controller",
		ImageName:          ThreeportGatewayControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
	},
}

var ThreeportRestApi *InstallInfo = &InstallInfo{
	Name:                "rest-api",
	ImageName:           ThreeportAPIImage,
	ImageRepo:           ThreeportImageRepo,
	ImageTag:            version.GetVersion(),
	ServiceAccountName:  "default",
	ServiceResourceName: ThreeportAPIServiceResourceName,
}

var ThreeportAgent *InstallInfo = &InstallInfo{
	Name:               "agent",
	ImageName:          ThreeportAgentImage,
	ImageRepo:          ThreeportImageRepo,
	ImageTag:           version.GetVersion(),
	ServiceAccountName: "default",
}

const (
	// The Kubernetes namespace in which the threeport control plane is
	// installed
	ControlPlaneNamespace = "threeport-control-plane"

	ControlPlaneName = "threeport"

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
