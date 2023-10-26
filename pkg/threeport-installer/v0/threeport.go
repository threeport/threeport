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
	ThreeportControlPlaneControllerImage      = "threeport-control-plane-controller"
	ThreeportAwsControllerImage               = "threeport-aws-controller"
	ThreeportGatewayControllerImage           = "threeport-gateway-controller"
	ThreeportAgentDeployName                  = "threeport-agent"
	ThreeportAgentImage                       = "threeport-agent"
	ThreeportAPIServiceResourceName           = "threeport-api-server"
	ThreeportLocalAPIEndpoint                 = "localhost"
	ThreeportWorkloadControllerName           = "workload-controller"
	ThreeportControlPlaneControllerName       = "control-plane-controller"
	ThreeportAwsControllerName                = "aws-controller"
)

var enabled bool = true

var ThreeportControllerList []*v0.ControlPlaneComponent = []*v0.ControlPlaneComponent{
	{
		Name:               ThreeportWorkloadControllerName,
		ImageName:          ThreeportWorkloadControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
		Enabled:            &enabled,
	},
	{
		Name:               "kubernetes-runtime-controller",
		ImageName:          ThreeportKubernetesRuntimeControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportAwsControllerName,
		ImageName:          ThreeportAwsControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
		Enabled:            &enabled,
	},
	{
		Name:               "gateway-controller",
		ImageName:          ThreeportGatewayControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportControlPlaneControllerName,
		ImageName:          ThreeportControlPlaneControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: "default",
		Enabled:            &enabled,
	},
}

var ThreeportRestApi *v0.ControlPlaneComponent = &v0.ControlPlaneComponent{
	Name:                "rest-api",
	ImageName:           ThreeportAPIImage,
	ImageRepo:           ThreeportImageRepo,
	ImageTag:            version.GetVersion(),
	ServiceAccountName:  "default",
	ServiceResourceName: ThreeportAPIServiceResourceName,
	Enabled:             &enabled,
}

var ThreeportAgent *v0.ControlPlaneComponent = &v0.ControlPlaneComponent{
	Name:               "agent",
	ImageName:          ThreeportAgentImage,
	ImageRepo:          ThreeportImageRepo,
	ImageTag:           version.GetVersion(),
	ServiceAccountName: "default",
	Enabled:            &enabled,
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
	// * Allow 15 chars for role names (defined in github.com/nukleros/aws-builder)
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
