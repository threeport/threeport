package v0

import (
	"github.com/threeport/threeport/internal/version"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

const (
	// Official image repo for threeport images
	ThreeportImageRepo = "ghcr.io/threeport"

	// Official image names for threeport control plane components
	ThreeportAPIImage                         = "threeport-rest-api"
	ThreeportDatabaseMigratorImage            = "threeport-database-migrator"
	ThreeportWorkloadControllerImage          = "threeport-workload-controller"
	ThreeportKubernetesRuntimeControllerImage = "threeport-kubernetes-runtime-controller"
	ThreeportControlPlaneControllerImage      = "threeport-control-plane-controller"
	ThreeportAwsControllerImage               = "threeport-aws-controller"
	ThreeportGatewayControllerImage           = "threeport-gateway-controller"
	ThreeportHelmWorkloadControllerImage      = "threeport-helm-workload-controller"
	ThreeportTerraformControllerImage         = "threeport-terraform-controller"
	ThreeportObservabilityControllerImage     = "threeport-observability-controller"
	ThreeportAgentImage                       = "threeport-agent"

	// Name of threeport control plane components
	ThreeportRestApiName                     = "rest-api"
	ThreeportDatabaseMigratorName            = "database-migrator"
	ThreeportWorkloadControllerName          = "workload-controller"
	ThreeportKubernetesRuntimeControllerName = "kubernetes-runtime-controller"
	ThreeportControlPlaneControllerName      = "control-plane-controller"
	ThreeportAwsControllerName               = "aws-controller"
	ThreeportGatewayControllerName           = "gateway-controller"
	ThreeportHelmWorkloadControllerName      = "helm-workload-controller"
	ThreeportObservabilityControllerName     = "observability-controller"
	ThreeportAgentName                       = "agent"

	// Endpoint for threeport API when running locally
	ThreeportLocalAPIEndpoint = "localhost"

	// Name of Kubernetes service resource for threeport API
	ThreeportAPIServiceResourceName = "threeport-api-server"

	// Name of Kubernetes deployment resource for threeport agent
	ThreeportAgentDeployName = "threeport-agent"

	// Name of default Kuberentes service account resource
	DefaultServiceAccount = "default"

	// Cockroach db image tag
	DatabaseImageTag = "v23.1.14"
)

var enabled bool = true

var ThreeportControllerList []*v0.ControlPlaneComponent = []*v0.ControlPlaneComponent{
	{
		Name:               ThreeportWorkloadControllerName,
		BinaryName:         ThreeportWorkloadControllerName,
		ImageName:          ThreeportWorkloadControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportKubernetesRuntimeControllerName,
		BinaryName:         ThreeportKubernetesRuntimeControllerName,
		ImageName:          ThreeportKubernetesRuntimeControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportAwsControllerName,
		BinaryName:         ThreeportAwsControllerName,
		ImageName:          ThreeportAwsControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportGatewayControllerName,
		BinaryName:         ThreeportGatewayControllerName,
		ImageName:          ThreeportGatewayControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportControlPlaneControllerName,
		BinaryName:         ThreeportControlPlaneControllerName,
		ImageName:          ThreeportControlPlaneControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportHelmWorkloadControllerName,
		BinaryName:         ThreeportHelmWorkloadControllerName,
		ImageName:          ThreeportHelmWorkloadControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportTerraformControllerName,
		BinaryName:         ThreeportTerraformControllerName,
		ImageName:          ThreeportTerraformControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
	{
		Name:               ThreeportObservabilityControllerName,
		BinaryName:         ThreeportObservabilityControllerName,
		ImageName:          ThreeportObservabilityControllerImage,
		ImageRepo:          ThreeportImageRepo,
		ImageTag:           version.GetVersion(),
		ServiceAccountName: DefaultServiceAccount,
		Enabled:            &enabled,
	},
}

var ThreeportRestApi *v0.ControlPlaneComponent = &v0.ControlPlaneComponent{
	Name:                ThreeportRestApiName,
	BinaryName:          ThreeportRestApiName,
	ImageName:           ThreeportAPIImage,
	ImageRepo:           ThreeportImageRepo,
	ImageTag:            version.GetVersion(),
	ServiceAccountName:  DefaultServiceAccount,
	ServiceResourceName: ThreeportAPIServiceResourceName,
	Enabled:             &enabled,
}

var ThreeportAgent *v0.ControlPlaneComponent = &v0.ControlPlaneComponent{
	Name:               ThreeportAgentName,
	BinaryName:         ThreeportAgentName,
	ImageName:          ThreeportAgentImage,
	ImageRepo:          ThreeportImageRepo,
	ImageTag:           version.GetVersion(),
	ServiceAccountName: DefaultServiceAccount,
	Enabled:            &enabled,
}

var DatabaseMigrator *v0.ControlPlaneComponent = &v0.ControlPlaneComponent{
	Name:       ThreeportDatabaseMigratorName,
	BinaryName: ThreeportDatabaseMigratorName,
	ImageName:  ThreeportDatabaseMigratorImage,
	ImageRepo:  ThreeportImageRepo,
	ImageTag:   version.GetVersion(),
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

// AllControlPlaneComponents returns a list of all control plane components.
func AllControlPlaneComponents() []*v0.ControlPlaneComponent {
	allControlPlaneComponents := ThreeportControllerList
	allControlPlaneComponents = append(allControlPlaneComponents, ThreeportRestApi)
	allControlPlaneComponents = append(allControlPlaneComponents, ThreeportAgent)
	return allControlPlaneComponents
}
