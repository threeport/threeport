package v0

import (
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

type InstallerOption func(o *Options)

type CustomInstallFunction func(*v0.KubernetesRuntimeInstance, *ControlPlaneInstaller) error

type Options struct {
	// Name of the control plane being installed, by default it is Threeport.
	Name string

	// Namespace of the control plane
	Namespace string

	// A function that is run prior to installing the components for the control plane.
	PreInstallFunction CustomInstallFunction

	// A function that is run after installing the components for the control plane.
	PostInstallFunction CustomInstallFunction

	// List of controllers to install as part of the control plane
	ControllerList []*v0.ControlPlaneComponent

	// Info for the Rest Api being installed
	RestApiInfo *v0.ControlPlaneComponent

	// Additionl init containers for rest api
	RestApiAdditionalInitContainers []map[string]interface{}

	// Info for the Database migrator being installed for the Rest Api
	DatabaseMigratorInfo *v0.ControlPlaneComponent

	// Info for the agent being installed
	AgentInfo *v0.ControlPlaneComponent

	// A boolean used to indicate whether the installer is being run from within threeport itself such as a reconciler
	InThreeport bool

	// CreateOrUpdate Kube resources during install. If true, resources will be updated if they already exist. If false, an error will occur if a resource already exists.
	CreateOrUpdateKubeResources bool

	// Installer option to determine if auth is enabled/disabled
	AuthEnabled bool

	// The AWS config profile to draw credentials from when using eks provider.
	AwsConfigProfile string

	// Retrieve AWS credentials from environment variables when using eks provider.
	AwsConfigEnv bool

	// AWS region code to install threeport control plane in.
	AwsRegion string

	// OCI region code to install threeport control plane in.
	OciRegion string

	// The OCI config profile to draw credentials from when using oke provider.
	OciConfigProfile string

	// The OCI compartment OCID to install threeport control plane in.
	OciCompartmentOcid string

	// The Google Cloud project ID where the cluster infra is provisioned.
	GcpProjectId string

	// The Google Cloud region where the cluster infra is provisioned.
	GcpRegion string

	// The GCP service account email for Workload Identity. When set, the
	// gcp-controller's Kubernetes ServiceAccount will be annotated to use
	// this GCP service account via Workload Identity.
	GcpServiceAccountEmail string

	// Path to config file for threeport
	CfgFile string

	// The root domain name to use for the Threeport API. Requires a public hosted zone in AWS Route53. A subdomain for the Threeport API will be added to the root domain.
	CreateRootDomain string

	// Email address of control plane admin. Provided to TLS provider.
	CreateAdminEmail string

	// Bool used to indicate whether installing in Dev environment or not
	DevEnvironment bool

	// EncryptionKey is the key used to encrypt and decrypt sensitive fields.
	EncryptionKey string

	// Overwrite any applicable config entries
	ForceOverwriteConfig bool

	// Name of the Control Plane being installed
	ControlPlaneName string

	// InfraProvider to instal control plane on e.g. kind, eks etc
	InfraProvider string

	// Path to kube config
	KubeconfigPath string

	// Number of additional worker nodes to deploy. Only applies to kind provider. (default is 0)
	NumWorkerNodes int

	// Path to infra provider config directory where cloud infra inventory is saved.
	ProviderConfigDir string

	// Path to threeport repository root
	ThreeportPath string

	// If true, run in debug mode. Appropriate for development environments only.
	Debug bool

	// If true, infrastructure is not provisioned, control plane is installed on existing infra.
	ControlPlaneOnly bool

	// Name of an existing kubernetes cluster to install the control plane on. Only applies
	// when ControlPlaneOnly is true.
	ClusterName string

	// If true, only infrastructure is provisioned.
	InfraOnly bool

	// Port mappings for kind infra provider
	KindPortMappings []string

	// If true, a cloud load balancer is provisioned for the threeport API.
	RestApiLoadBalancer bool

	// verbose logging
	Verbose bool

	// provide any additional conditions to be added to aws IRSA
	AdditionalAwsIrsaConditions []string

	// A general map to pass around information between various install phases.
	AdditionalOptions map[string]interface{}

	// Automatically tear down control plane resources if an error is encountered.
	TeardownOnFailure bool

	// Create and connect local container registry for local control plane
	// clusters.
	LocalRegistry bool

	// PaginationMode sets the REST API server's pagination strategy through
	// the `-pagination-mode` flag. Accepted values are `as-of-system-time`
	// and `materialized-view`. A nil or empty value omits the flag, so the
	// API server applies its own default.
	PaginationMode *string
}

type ControlPlaneInstaller struct {
	Opts Options
}

func (cpi *ControlPlaneInstaller) SetAllImageRepo(imageRepo string) {
	for _, c := range cpi.Opts.ControllerList {
		c.ImageNamespace = imageRepo
	}
	cpi.Opts.RestApiInfo.ImageNamespace = imageRepo
	cpi.Opts.AgentInfo.ImageNamespace = imageRepo
	cpi.Opts.DatabaseMigratorInfo.ImageNamespace = imageRepo
}

func (cpi *ControlPlaneInstaller) SetAllImageTags(imageTag string) {
	for _, c := range cpi.Opts.ControllerList {
		c.ImageTag = imageTag
	}
	cpi.Opts.RestApiInfo.ImageTag = imageTag
	cpi.Opts.AgentInfo.ImageTag = imageTag
	cpi.Opts.DatabaseMigratorInfo.ImageTag = imageTag
}

func Name(n string) InstallerOption {
	return func(o *Options) {
		o.Name = n
	}
}

func Namespace(n string) InstallerOption {
	return func(o *Options) {
		o.Namespace = n
	}
}

func RestApi(r *v0.ControlPlaneComponent) InstallerOption {
	return func(o *Options) {
		o.RestApiInfo = r
	}
}

func CustomController(c *v0.ControlPlaneComponent) InstallerOption {
	return func(o *Options) {
		o.ControllerList = append(o.ControllerList, c)
	}
}

func CustomControllers(c []*v0.ControlPlaneComponent) InstallerOption {
	return func(o *Options) {
		o.ControllerList = append(o.ControllerList, c...)
	}
}

func PreInstallFunction(f CustomInstallFunction) InstallerOption {
	return func(o *Options) {
		o.PreInstallFunction = f
	}
}

func PostInstallFunction(f CustomInstallFunction) InstallerOption {
	return func(o *Options) {
		o.PostInstallFunction = f
	}
}

func defaultInstallFunction(kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance, cpi *ControlPlaneInstaller) error {
	return nil
}

var defaultInstallerOptions = Options{
	Name:                        ControlPlaneName,
	Namespace:                   ControlPlaneNamespace,
	ControllerList:              ThreeportControllerList,
	RestApiInfo:                 ThreeportRestApi,
	DatabaseMigratorInfo:        DatabaseMigrator,
	AgentInfo:                   ThreeportAgent,
	PreInstallFunction:          defaultInstallFunction,
	PostInstallFunction:         defaultInstallFunction,
	InThreeport:                 false,
	AdditionalAwsIrsaConditions: make([]string, 0),
	AdditionalOptions:           make(map[string]interface{}),
}

func NewInstaller(os ...InstallerOption) *ControlPlaneInstaller {
	opts := &defaultInstallerOptions
	for _, o := range os {
		o(opts)
	}

	return &ControlPlaneInstaller{
		Opts: *opts,
	}
}
