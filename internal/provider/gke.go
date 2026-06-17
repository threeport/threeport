package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	container "cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	gkecontainer "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/iam/v1"
	gcpiam "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	gcpoption "google.golang.org/api/option"
	"gorm.io/datatypes"

	gcpauth "github.com/threeport/threeport/pkg/auth/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// KubernetesRuntimeInfraGKE represents the infrastructure for a threeport-managed
// GKE (Google Kubernetes Engine) cluster.
type KubernetesRuntimeInfraGKE struct {
	// PulumiWorkspace provides workspace, stack, state, and automation API helpers
	// (aligned with KubernetesRuntimeInfraOKE).
	PulumiWorkspace

	// Kubernetes version of the GKE cluster.
	Version string

	// The Google Cloud project ID where the cluster infra is provisioned.
	ProjectID string

	// The Google Cloud region where the cluster infra is provisioned.
	Region string

	// The number of nodes initially created for the worker node pool.
	WorkerNodeInitialCount int32

	// The email address of the GCP service account created for Threeport.
	// This service account is used with Workload Identity to allow
	// Threeport controllers to manage GCP resources.
	ServiceAccountEmail string

	// ServiceAccountCredentials contains the JSON key for a GCP service account.
	// Used when running outside GCP (e.g., in AWS or on-prem) where Workload
	// Identity is not available. This is the contents of a service account key
	// file exported from GCP Console.
	ServiceAccountCredentials string

	// credentialsFilePath stores the path to a temporary file containing
	// the service account credentials. This is set internally when
	// ServiceAccountCredentials is used.
	credentialsFilePath string

	// gcpProjectResolvedFromGcloudINI is true when loadGCPConfig set ProjectID from the active gcloud INI.
	gcpProjectResolvedFromGcloudINI bool
	// gcpRegionResolvedFromGcloudINI is true when loadGCPConfig set Region from the active gcloud INI.
	gcpRegionResolvedFromGcloudINI bool
}

// ensurePulumiProjectDefaults sets Pulumi project metadata when not provided by callers.
func (i *KubernetesRuntimeInfraGKE) ensurePulumiProjectDefaults() {
	if i.ProjectName == "" {
		i.ProjectName = "gke"
	}
	if i.ProjectDescription == "" {
		i.ProjectDescription = "Google Kubernetes Engine (GKE) cluster for Threeport"
	}
}

// syncStackConfigs updates stack config keys from the current ProjectID and Region.
func (i *KubernetesRuntimeInfraGKE) syncStackConfigs() {
	i.StackConfigs = map[string]string{
		"gcp:project": i.ProjectID,
		"gcp:region":  i.Region,
	}
}

// Create installs a Kubernetes cluster using Google Cloud GKE for threeport workloads.
func (i *KubernetesRuntimeInfraGKE) Create() (*kube.KubeConnectionInfo, error) {
	// ensure GCP authentication is in place
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return nil, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	if i.ServiceAccountCredentials != "" {
		defer gcpauth.CleanupGCPCredentials()
	}

	// load GCP configuration to ensure ProjectID is set
	if err := i.loadGCPConfig(); err != nil {
		return nil, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	// Only compare ADC identity to gcloud core.account when project or region came from the
	// gcloud profile; explicit flags/env avoid implying that profile's user context.
	if i.gcpProjectResolvedFromGcloudINI || i.gcpRegionResolvedFromGcloudINI {
		if err := validateADCUserMatchesGcloudAccount(context.Background()); err != nil {
			return nil, err
		}
	}

	// create GCP service account for Threeport to manage GCP resources
	// This service account will be used with Workload Identity
	if err := i.createGCPServiceAccountAndCredentials(); err != nil {
		return nil, fmt.Errorf("failed to create GCP service account: %w", err)
	}

	return i.CreateInfra()
}

// CreateInfra provisions GKE cluster infrastructure via Pulumi (network, cluster, node pool)
// and configures Workload Identity. It does not create the Threeport GCP service account —
// call createGCPServiceAccountAndCredentials first for the bootstrap path, or ensure
// credentials and Workload Identity are already configured for the controller path.
func (i *KubernetesRuntimeInfraGKE) CreateInfra() (*kube.KubeConnectionInfo, error) {
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return nil, fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	ctx := context.Background()
	if _, err = i.RunUp(ctx, stack); err != nil {
		return nil, fmt.Errorf("failed to deploy stack: %w", err)
	}

	// configure Workload Identity binding after cluster is created
	// This allows Kubernetes service accounts to impersonate the GCP service account
	if err := i.configureWorkloadIdentityBindingPostCreate(); err != nil {
		return nil, fmt.Errorf("failed to configure Workload Identity binding: %w", err)
	}

	return i.GetConnection()
}

// DeployInfra runs CreateInfra and discards connection info. It satisfies InfraProvider
// for the shared infrastructure lifecycle (same pattern as KubernetesRuntimeInfraOKE).
func (i *KubernetesRuntimeInfraGKE) DeployInfra() error {
	_, err := i.CreateInfra()
	return err
}

// DestroyInfra tears down GKE infrastructure via Delete. It satisfies InfraProvider.
func (i *KubernetesRuntimeInfraGKE) DestroyInfra() error {
	return i.Delete()
}

// pulumiProgram defines the Pulumi resources for the GKE stack.
func (i *KubernetesRuntimeInfraGKE) pulumiProgram() pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		gcpProvider, err := gcp.NewProvider(ctx, "gcp-provider", &gcp.ProviderArgs{
			Project: pulumi.String(i.ProjectID),
			Region:  pulumi.String(i.Region),
		})
		if err != nil {
			return fmt.Errorf("failed to create GCP provider: %w", err)
		}

		network, err := compute.NewNetwork(ctx, fmt.Sprintf("%s-vpc", i.RuntimeInstanceName), &compute.NetworkArgs{
			Name:                  pulumi.String(fmt.Sprintf("%s-vpc", i.RuntimeInstanceName)),
			AutoCreateSubnetworks: pulumi.Bool(false),
			Description:           pulumi.String(fmt.Sprintf("VPC network for Threeport GKE cluster %s", i.RuntimeInstanceName)),
		}, pulumi.Provider(gcpProvider))
		if err != nil {
			return fmt.Errorf("failed to create VPC network: %w", err)
		}

		subnet, err := compute.NewSubnetwork(ctx, fmt.Sprintf("%s-subnet", i.RuntimeInstanceName), &compute.SubnetworkArgs{
			Name:        pulumi.String(fmt.Sprintf("%s-subnet", i.RuntimeInstanceName)),
			IpCidrRange: pulumi.String("10.0.0.0/16"),
			Region:      pulumi.String(i.Region),
			Network:     network.ID(),
			SecondaryIpRanges: compute.SubnetworkSecondaryIpRangeArray{
				&compute.SubnetworkSecondaryIpRangeArgs{
					RangeName:   pulumi.String("pods"),
					IpCidrRange: pulumi.String("10.1.0.0/16"),
				},
				&compute.SubnetworkSecondaryIpRangeArgs{
					RangeName:   pulumi.String("services"),
					IpCidrRange: pulumi.String("10.2.0.0/20"),
				},
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return fmt.Errorf("failed to create subnet: %w", err)
		}

		router, err := compute.NewRouter(ctx, fmt.Sprintf("%s-router", i.RuntimeInstanceName), &compute.RouterArgs{
			Name:    pulumi.String(fmt.Sprintf("%s-router", i.RuntimeInstanceName)),
			Network: network.ID(),
			Region:  pulumi.String(i.Region),
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return fmt.Errorf("failed to create Cloud Router: %w", err)
		}

		_, err = compute.NewRouterNat(ctx, fmt.Sprintf("%s-nat", i.RuntimeInstanceName), &compute.RouterNatArgs{
			Name:                          pulumi.String(fmt.Sprintf("%s-nat", i.RuntimeInstanceName)),
			Router:                        router.Name,
			Region:                        pulumi.String(i.Region),
			NatIpAllocateOption:           pulumi.String("AUTO_ONLY"),
			SourceSubnetworkIpRangesToNat: pulumi.String("ALL_SUBNETWORKS_ALL_IP_RANGES"),
			LogConfig: &compute.RouterNatLogConfigArgs{
				Enable: pulumi.Bool(true),
				Filter: pulumi.String("ERRORS_ONLY"),
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{router}))
		if err != nil {
			return fmt.Errorf("failed to create Cloud NAT: %w", err)
		}

		cluster, err := gkecontainer.NewCluster(ctx, i.RuntimeInstanceName, &gkecontainer.ClusterArgs{
			Name:       pulumi.String(i.RuntimeInstanceName),
			Location:   pulumi.String(i.Region),
			Network:    network.Name,
			Subnetwork: subnet.Name,

			IpAllocationPolicy: &gkecontainer.ClusterIpAllocationPolicyArgs{
				ClusterSecondaryRangeName:  pulumi.String("pods"),
				ServicesSecondaryRangeName: pulumi.String("services"),
			},

			InitialNodeCount:      pulumi.Int(1),
			RemoveDefaultNodePool: pulumi.Bool(true),

			PrivateClusterConfig: &gkecontainer.ClusterPrivateClusterConfigArgs{
				EnablePrivateNodes:    pulumi.Bool(true),
				EnablePrivateEndpoint: pulumi.Bool(false),
				MasterIpv4CidrBlock:   pulumi.String("172.16.0.0/28"),
			},

			NetworkPolicy: &gkecontainer.ClusterNetworkPolicyArgs{
				Enabled:  pulumi.Bool(true),
				Provider: pulumi.String("CALICO"),
			},

			WorkloadIdentityConfig: &gkecontainer.ClusterWorkloadIdentityConfigArgs{
				WorkloadPool: pulumi.String(fmt.Sprintf("%s.svc.id.goog", i.ProjectID)),
			},

			DeletionProtection: pulumi.Bool(false),
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{subnet}))
		if err != nil {
			return fmt.Errorf("failed to create GKE cluster: %w", err)
		}

		_, err = gkecontainer.NewNodePool(ctx, fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName), &gkecontainer.NodePoolArgs{
			Name:     pulumi.String(fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName)),
			Cluster:  cluster.Name,
			Location: pulumi.String(i.Region),

			NodeCount: pulumi.Int(int(i.WorkerNodeInitialCount)),

			NodeConfig: &gkecontainer.NodePoolNodeConfigArgs{
				MachineType: pulumi.String("e2-medium"),
				DiskSizeGb:  pulumi.Int(50),
				DiskType:    pulumi.String("pd-standard"),
				OauthScopes: pulumi.StringArray{
					pulumi.String("https://www.googleapis.com/auth/cloud-platform"),
				},
				Labels: pulumi.StringMap{
					kube.ThreeportManagedByLabelKey: pulumi.String(kube.ThreeportManagedByLabelValue),
				},
				WorkloadMetadataConfig: &gkecontainer.NodePoolNodeConfigWorkloadMetadataConfigArgs{
					Mode: pulumi.String("GKE_METADATA"),
				},
			},

			Autoscaling: &gkecontainer.NodePoolAutoscalingArgs{
				MinNodeCount:   pulumi.Int(1),
				MaxNodeCount:   pulumi.Int(10),
				LocationPolicy: pulumi.String("ANY"),
			},

			Management: &gkecontainer.NodePoolManagementArgs{
				AutoRepair:  pulumi.Bool(true),
				AutoUpgrade: pulumi.Bool(true),
			},
		}, pulumi.Provider(gcpProvider),
			pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return fmt.Errorf("failed to create node pool: %w", err)
		}

		return nil
	}
}

// Delete deletes a GKE cluster and the threeport control plane with it.
func (i *KubernetesRuntimeInfraGKE) Delete() error {
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	if i.ServiceAccountCredentials != "" {
		defer gcpauth.CleanupGCPCredentials()
	}

	if err := i.loadGCPConfig(); err != nil {
		return fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	if err := i.DestroyStack(); err != nil {
		return fmt.Errorf("failed to destroy Pulumi stack: %w", err)
	}

	if err := i.DeleteGCPResources(); err != nil {
		if i.Logger != nil {
			i.Logger.Info("warning: failed to clean up GCP resources", "error", err.Error())
		} else {
			util.CliOutputWarning(fmt.Sprintf("failed to clean up GCP resources: %v", err))
		}
	}

	return nil
}

// DeleteGCPResources deletes all GCP resources created for this Threeport instance
// that are not managed by Pulumi.
func (i *KubernetesRuntimeInfraGKE) DeleteGCPResources() error {
	ctx := context.Background()

	// Create IAM service client
	iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	// Create Cloud Resource Manager service client for IAM bindings
	crmService, err := cloudresourcemanager.NewService(ctx, option.WithScopes(cloudresourcemanager.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}

	// Remove IAM role bindings
	if err := i.removeServiceAccountRoles(crmService); err != nil {
		return fmt.Errorf("failed to remove IAM roles: %w", err)
	}

	// Delete the service account
	if err := i.deleteGCPServiceAccount(iamService); err != nil {
		return fmt.Errorf("failed to delete service account: %w", err)
	}

	return nil
}

// GetConnection returns the connection information for the GKE cluster.
func (i *KubernetesRuntimeInfraGKE) GetConnection() (*kube.KubeConnectionInfo, error) {
	// ensure GCP authentication is in place
	if err := gcpauth.EnsureGCPAuth(i.ServiceAccountCredentials); err != nil {
		return nil, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}
	if i.ServiceAccountCredentials != "" {
		defer gcpauth.CleanupGCPCredentials()
	}

	// load GCP configuration from gcloud CLI config or environment variables
	if err := i.loadGCPConfig(); err != nil {
		return nil, fmt.Errorf("failed to load GCP configuration: %w", err)
	}

	ctx := context.Background()

	// create GKE cluster manager client
	// This uses Application Default Credentials (ADC)
	clusterManagerClient, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster manager client: %w", err)
	}
	defer clusterManagerClient.Close()

	// construct the cluster name in the format required by GKE API
	// Format: projects/{project}/locations/{location}/clusters/{cluster}
	clusterName := fmt.Sprintf(
		"projects/%s/locations/%s/clusters/%s",
		i.ProjectID, i.Region, i.RuntimeInstanceName,
	)

	// get cluster details
	cluster, err := clusterManagerClient.GetCluster(ctx, &containerpb.GetClusterRequest{
		Name: clusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster details: %w", err)
	}

	// decode the CA certificate
	caCert, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA certificate: %w", err)
	}

	// get an access token for authentication
	// TODO: Implement token refresh mechanism for long-running operations
	// The token has a limited lifetime (typically 1 hour)
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to get token source: %w", err)
	}

	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// construct the API endpoint
	// GKE provides the endpoint without the https:// prefix
	apiEndpoint := fmt.Sprintf("https://%s", cluster.Endpoint)

	// create connection info
	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:     apiEndpoint,
		CACertificate:   string(caCert),
		Token:           token.AccessToken,
		TokenExpiration: token.Expiry,
	}

	return kubeConnInfo, nil
}

// LoadConfigFromStack loads the GCP project ID and region from the existing
// Pulumi stack configuration. This is useful when deleting a cluster where
// we only have the runtime instance name but need the project and region
// to connect to GCP.
func (i *KubernetesRuntimeInfraGKE) LoadConfigFromStack() error {
	i.ensurePulumiProjectDefaults()

	workspace, err := i.initWorkspace(auto.Program(func(ctx *pulumi.Context) error { return nil }))
	if err != nil {
		return fmt.Errorf("failed to initialize Pulumi workspace: %w", err)
	}

	ctx := context.Background()
	stack, err := auto.SelectStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return fmt.Errorf("failed to select stack: %w", err)
	}

	projectConfig, err := stack.GetConfig(ctx, "gcp:project")
	if err != nil {
		return fmt.Errorf("failed to get gcp:project config: %w", err)
	}
	i.ProjectID = projectConfig.Value

	regionConfig, err := stack.GetConfig(ctx, "gcp:region")
	if err != nil {
		return fmt.Errorf("failed to get gcp:region config: %w", err)
	}
	i.Region = regionConfig.Value

	return nil
}

// GetStackState returns the state of the GKE stack as a JSON object.
func (i *KubernetesRuntimeInfraGKE) GetStackState() (*datatypes.JSON, error) {
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()
	return i.PulumiWorkspace.GetStackState()
}

// SetStackState restores Pulumi state from a JSON object (export or checkpoint format).
func (i *KubernetesRuntimeInfraGKE) SetStackState(state *datatypes.JSON) error {
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()
	return i.PulumiWorkspace.SetStackState(state)
}

// configureWorkloadIdentityBindingPostCreate sets up the Workload Identity binding
// after the GKE cluster has been created.
func (i *KubernetesRuntimeInfraGKE) configureWorkloadIdentityBindingPostCreate() error {
	ctx := context.Background()

	// Create IAM service client
	iamService, err := gcpiam.NewService(ctx, gcpoption.WithScopes(gcpiam.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	return i.configureWorkloadIdentityBinding(iamService)
}

// loadGCPConfig populates ProjectID and Region when they are not already set.
// Priority for each field independently:
//  1. Caller-provided values (e.g. CLI flags) already on the receiver — never overwritten
//  2. Environment variables (only if that field is still empty)
//  3. Active gcloud configuration INI ([core] project, [compute] region)
func (i *KubernetesRuntimeInfraGKE) loadGCPConfig() error {
	i.gcpProjectResolvedFromGcloudINI = false
	i.gcpRegionResolvedFromGcloudINI = false

	if i.ProjectID == "" {
		projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			projectID = os.Getenv("CLOUDSDK_CORE_PROJECT")
		}
		if projectID == "" {
			projectID = os.Getenv("GCLOUD_PROJECT")
		}
		if projectID != "" {
			i.ProjectID = projectID
		}
	}

	if i.Region == "" {
		region := os.Getenv("CLOUDSDK_COMPUTE_REGION")
		if region == "" {
			region = os.Getenv("GOOGLE_REGION")
		}
		if region != "" {
			i.Region = region
		}
	}

	if i.ProjectID != "" && i.Region != "" {
		return nil
	}

	if err := i.loadGCPConfigFromFile(); err != nil {
		if i.ProjectID == "" {
			return fmt.Errorf("GCP project ID not found in environment variables or gcloud config: %w", err)
		}
		if i.Region == "" {
			return fmt.Errorf("GCP region not found in environment variables or gcloud config: %w", err)
		}
	}

	return nil
}

// loadGCPConfigFromFile reads the gcloud CLI configuration files
// to get project ID and region settings.
func (i *KubernetesRuntimeInfraGKE) loadGCPConfigFromFile() error {
	cfg, err := loadActiveGcloudConfigINI()
	if err != nil {
		return err
	}

	// get project ID from [core] section
	if i.ProjectID == "" {
		projectID := cfg.Section("core").Key("project").String()
		if projectID != "" {
			i.ProjectID = projectID
			i.gcpProjectResolvedFromGcloudINI = true
		}
	}

	// get region from [compute] section
	if i.Region == "" {
		region := cfg.Section("compute").Key("region").String()
		if region != "" {
			i.Region = region
			i.gcpRegionResolvedFromGcloudINI = true
		}
	}

	return nil
}

