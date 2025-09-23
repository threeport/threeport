package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	ociContainerEngine "github.com/oracle/oci-go-sdk/v65/containerengine"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci/containerengine"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci/core"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// OCIInfrastructurePulumi handles Stage 2 OKE infrastructure deployment
type OCIInfrastructurePulumi struct {
	RuntimeInstanceName    string
	Version                string
	CompartmentOCID        string
	TenancyOCID            string
	TargetRegion           string
	WorkerNodeShape        string
	WorkerNodeInitialCount int32
	bootstrapOutputs       *BootstrapOutputs
}

// InfrastructureOutputs represents the outputs from Stage 2 infrastructure stack
type InfrastructureOutputs struct {
	ClusterID   string
	NodePoolID  string
	ClusterName string
	KubeConfig  string
}

// NewOCIInfrastructurePulumi creates a new infrastructure deployment instance
func NewOCIInfrastructurePulumi(runtimeInstanceName, version, compartmentOCID, tenancyOCID, targetRegion, workerNodeShape string, workerNodeInitialCount int32, bootstrapOutputs *BootstrapOutputs) *OCIInfrastructurePulumi {
	return &OCIInfrastructurePulumi{
		RuntimeInstanceName:    runtimeInstanceName,
		Version:                version,
		CompartmentOCID:        compartmentOCID,
		TenancyOCID:            tenancyOCID,
		TargetRegion:           targetRegion,
		WorkerNodeShape:        workerNodeShape,
		WorkerNodeInitialCount: workerNodeInitialCount,
		bootstrapOutputs:       bootstrapOutputs,
	}
}

// getServiceGatewayID gets the service ID for all services in the target region
func (i *OCIInfrastructurePulumi) getServiceGatewayID() (string, string, error) {
	configProvider := common.DefaultConfigProvider()
	networkClient, err := ociCore.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", "", fmt.Errorf("failed to create network client: %v", err)
	}

	// Set target region for the client
	networkClient.SetRegion(i.TargetRegion)

	// List services
	request := ociCore.ListServicesRequest{}
	response, err := networkClient.ListServices(context.Background(), request)
	if err != nil {
		return "", "", fmt.Errorf("failed to list services: %v", err)
	}

	// Find the "all services" entry using regex pattern
	pattern := `^All [A-Z]+ Services In Oracle Services Network$`
	re := regexp.MustCompile(pattern)
	for _, service := range response.Items {
		if service.Description != nil && re.MatchString(*service.Description) {
			return *service.Id, *service.CidrBlock, nil
		}
	}

	return "", "", fmt.Errorf("failed to find all services entry in region %s", i.TargetRegion)
}

// getOKEWorkerNodeImageOCID returns the OCID of the latest OKE worker node image
// with version specified in struct
func (i *OCIInfrastructurePulumi) getOKEWorkerNodeImageOCID() (string, error) {
	configProvider := common.DefaultConfigProvider()
	containerClient, err := ociContainerEngine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create container engine client: %v", err)
	}

	// Set target region for the client
	containerClient.SetRegion(i.TargetRegion)

	// Create a request to list node pool options
	request := ociContainerEngine.GetNodePoolOptionsRequest{
		CompartmentId:    common.String(i.CompartmentOCID),
		NodePoolOptionId: common.String("all"),
	}

	// Call the API to get node pool options
	response, err := containerClient.GetNodePoolOptions(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to get node pool options: %v", err)
	}

	// Check if we have any images
	if len(response.Sources) == 0 {
		return "", fmt.Errorf("no OKE worker node images found")
	}

	// Find an image with the specified Kubernetes version
	for _, source := range response.Sources {
		// Try to get the concrete type
		if sourceType, ok := source.(ociContainerEngine.NodeSourceViaImageOption); ok {
			name := *sourceType.SourceName
			// Remove leading 'v' from version for image search
			versionWithoutV := strings.TrimPrefix(i.Version, "v")
			if strings.Contains(name, fmt.Sprintf("OKE-%s", versionWithoutV)) &&
				strings.Contains(name, "aarch64") {
				return *sourceType.ImageId, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable OKE worker node images found with aarch64 architecture and Kubernetes version %s", i.Version)
}

// getAvailabilityDomain returns the first available availability domain in the target region
func (i *OCIInfrastructurePulumi) getAvailabilityDomain() (string, error) {
	configProvider := common.DefaultConfigProvider()
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create identity client: %v", err)
	}

	// Set target region for the client
	identityClient.SetRegion(i.TargetRegion)

	// Create a request to list availability domains
	request := identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(i.CompartmentOCID),
	}

	// Call the API to get availability domains
	response, err := identityClient.ListAvailabilityDomains(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to list availability domains: %v", err)
	}

	// Check if we have any availability domains
	if len(response.Items) == 0 {
		return "", fmt.Errorf("no availability domains found in region %s", i.TargetRegion)
	}

	// Return the first availability domain
	return *response.Items[0].Name, nil
}

// infrastructurePulumiProgram defines the Pulumi program for Stage 2 infrastructure
func (i *OCIInfrastructurePulumi) infrastructurePulumiProgram(ctx *pulumi.Context) error {
	// Create OCI provider with target region (credentials come from THREEPORT_SERVICE profile)
	ociProvider, err := oci.NewProvider(ctx, "oci-provider", &oci.ProviderArgs{
		Region: pulumi.String(i.TargetRegion),
	})
	if err != nil {
		return fmt.Errorf("failed to create OCI provider: %v", err)
	}

	// Create VCN
	vcn, err := core.NewVcn(ctx, fmt.Sprintf("%s-vcn", i.RuntimeInstanceName), &core.VcnArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		CidrBlocks:    pulumi.StringArray{pulumi.String("10.0.0.0/16")},
		DisplayName:   pulumi.String(fmt.Sprintf("%s-vcn", i.RuntimeInstanceName)),
		DnsLabel:      pulumi.String("threeportvcn"),
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create VCN: %v", err)
	}

	// Create Internet Gateway
	internetGateway, err := core.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", i.RuntimeInstanceName), &core.InternetGatewayArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-igw", i.RuntimeInstanceName)),
		Enabled:       pulumi.Bool(true),
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create internet gateway: %v", err)
	}

	// Create NAT Gateway
	natGateway, err := core.NewNatGateway(ctx, fmt.Sprintf("%s-natgw", i.RuntimeInstanceName), &core.NatGatewayArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-natgw", i.RuntimeInstanceName)),
		BlockTraffic:  pulumi.Bool(false),
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create NAT gateway: %v", err)
	}

	// Get service gateway ID and CIDR block
	serviceID, serviceCIDR, err := i.getServiceGatewayID()
	if err != nil {
		return fmt.Errorf("failed to get service gateway ID: %v", err)
	}

	// Create Service Gateway
	serviceGateway, err := core.NewServiceGateway(ctx, fmt.Sprintf("%s-servicegw", i.RuntimeInstanceName), &core.ServiceGatewayArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-servicegw", i.RuntimeInstanceName)),
		Services: core.ServiceGatewayServiceArray{
			&core.ServiceGatewayServiceArgs{
				ServiceId: pulumi.String(serviceID),
			},
		},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create service gateway: %v", err)
	}

	// Create Public Route Table
	publicRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName)),
		RouteRules: core.RouteTableRouteRuleArray{
			&core.RouteTableRouteRuleArgs{
				NetworkEntityId: internetGateway.ID(),
				Destination:     pulumi.String("0.0.0.0/0"),
				DestinationType: pulumi.String("CIDR_BLOCK"),
			},
		},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create public route table: %v", err)
	}

	// Create Private Route Table
	privateRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-private-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-private-rt", i.RuntimeInstanceName)),
		RouteRules: core.RouteTableRouteRuleArray{
			&core.RouteTableRouteRuleArgs{
				NetworkEntityId: natGateway.ID(),
				Destination:     pulumi.String("0.0.0.0/0"),
				DestinationType: pulumi.String("CIDR_BLOCK"),
			},
			&core.RouteTableRouteRuleArgs{
				NetworkEntityId: serviceGateway.ID(),
				Destination:     pulumi.String(serviceCIDR),
				DestinationType: pulumi.String("SERVICE_CIDR_BLOCK"),
			},
		},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create private route table: %v", err)
	}

	// Create Security Lists
	workerSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-worker-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-worker-seclist", i.RuntimeInstanceName)),
		EgressSecurityRules: core.SecurityListEgressSecurityRuleArray{
			&core.SecurityListEgressSecurityRuleArgs{
				Protocol:    pulumi.String("all"),
				Destination: pulumi.String("0.0.0.0/0"),
				Stateless:   pulumi.Bool(false),
			},
		},
		IngressSecurityRules: core.SecurityListIngressSecurityRuleArray{
			&core.SecurityListIngressSecurityRuleArgs{
				Protocol: pulumi.String("6"),
				Source:   pulumi.String("10.0.10.0/24"),
				TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
					Min: pulumi.Int(22),
					Max: pulumi.Int(22),
				},
				Stateless: pulumi.Bool(false),
			},
			&core.SecurityListIngressSecurityRuleArgs{
				Protocol:  pulumi.String("6"),
				Source:    pulumi.String("10.0.0.0/16"),
				Stateless: pulumi.Bool(false),
			},
		},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create worker security list: %v", err)
	}

	loadBalancerSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-lb-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
		CompartmentId: pulumi.String(i.CompartmentOCID),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-lb-seclist", i.RuntimeInstanceName)),
		EgressSecurityRules: core.SecurityListEgressSecurityRuleArray{
			&core.SecurityListEgressSecurityRuleArgs{
				Protocol:    pulumi.String("6"),
				Destination: pulumi.String("10.0.20.0/24"),
				Stateless:   pulumi.Bool(false),
			},
		},
		IngressSecurityRules: core.SecurityListIngressSecurityRuleArray{
			&core.SecurityListIngressSecurityRuleArgs{
				Protocol: pulumi.String("6"),
				Source:   pulumi.String("0.0.0.0/0"),
				TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
					Min: pulumi.Int(80),
					Max: pulumi.Int(80),
				},
				Stateless: pulumi.Bool(false),
			},
			&core.SecurityListIngressSecurityRuleArgs{
				Protocol: pulumi.String("6"),
				Source:   pulumi.String("0.0.0.0/0"),
				TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
					Min: pulumi.Int(443),
					Max: pulumi.Int(443),
				},
				Stateless: pulumi.Bool(false),
			},
		},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create load balancer security list: %v", err)
	}

	// Create Subnets
	publicSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
		CompartmentId:           pulumi.String(i.CompartmentOCID),
		VcnId:                   vcn.ID(),
		CidrBlock:               pulumi.String("10.0.10.0/24"),
		DisplayName:             pulumi.String(fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName)),
		DnsLabel:                pulumi.String("publicsubnet"),
		ProhibitInternetIngress: pulumi.Bool(false),
		ProhibitPublicIpOnVnic:  pulumi.Bool(false),
		RouteTableId:            publicRouteTable.ID(),
		SecurityListIds:         pulumi.StringArray{workerSecList.ID()},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create public subnet: %v", err)
	}

	privateSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
		CompartmentId:           pulumi.String(i.CompartmentOCID),
		VcnId:                   vcn.ID(),
		CidrBlock:               pulumi.String("10.0.20.0/24"),
		DisplayName:             pulumi.String(fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName)),
		DnsLabel:                pulumi.String("privatesubnet"),
		ProhibitInternetIngress: pulumi.Bool(true),
		ProhibitPublicIpOnVnic:  pulumi.Bool(true),
		RouteTableId:            privateRouteTable.ID(),
		SecurityListIds:         pulumi.StringArray{workerSecList.ID()},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create private subnet: %v", err)
	}

	loadBalancerSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-lb-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
		CompartmentId:           pulumi.String(i.CompartmentOCID),
		VcnId:                   vcn.ID(),
		CidrBlock:               pulumi.String("10.0.30.0/24"),
		DisplayName:             pulumi.String(fmt.Sprintf("%s-lb-subnet", i.RuntimeInstanceName)),
		DnsLabel:                pulumi.String("lbsubnet"),
		ProhibitInternetIngress: pulumi.Bool(false),
		ProhibitPublicIpOnVnic:  pulumi.Bool(false),
		RouteTableId:            publicRouteTable.ID(),
		SecurityListIds:         pulumi.StringArray{loadBalancerSecList.ID()},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create load balancer subnet: %v", err)
	}

	// Create OKE Cluster
	cluster, err := containerengine.NewCluster(ctx, i.RuntimeInstanceName, &containerengine.ClusterArgs{
		CompartmentId:     pulumi.String(i.CompartmentOCID),
		Name:              pulumi.String(i.RuntimeInstanceName),
		VcnId:             vcn.ID(),
		KubernetesVersion: pulumi.String(i.Version),
		EndpointConfig: &containerengine.ClusterEndpointConfigArgs{
			IsPublicIpEnabled: pulumi.Bool(true),
			SubnetId:          publicSubnet.ID(),
			NsgIds:            pulumi.StringArray{},
		},
		Options: &containerengine.ClusterOptionsArgs{
			KubernetesNetworkConfig: &containerengine.ClusterOptionsKubernetesNetworkConfigArgs{
				PodsCidr:     pulumi.String("10.244.0.0/16"),
				ServicesCidr: pulumi.String("10.96.0.0/16"),
			},
			ServiceLbSubnetIds: pulumi.StringArray{loadBalancerSubnet.ID()},
		},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create OKE cluster: %v", err)
	}

	// Get the OKE worker node image OCID
	imageOCID, err := i.getOKEWorkerNodeImageOCID()
	if err != nil {
		return fmt.Errorf("failed to get OKE worker node image OCID: %v", err)
	}

	// Get the first available availability domain
	availabilityDomain, err := i.getAvailabilityDomain()
	if err != nil {
		return fmt.Errorf("failed to get availability domain: %v", err)
	}

	// Create Node Pool
	nodePool, err := containerengine.NewNodePool(ctx, fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName), &containerengine.NodePoolArgs{
		ClusterId:         cluster.ID(),
		CompartmentId:     pulumi.String(i.CompartmentOCID),
		Name:              pulumi.String(fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName)),
		NodeShape:         pulumi.String(i.WorkerNodeShape),
		KubernetesVersion: pulumi.String(i.Version),
		InitialNodeLabels: containerengine.NodePoolInitialNodeLabelArray{
			&containerengine.NodePoolInitialNodeLabelArgs{
				Key:   pulumi.String("threeport.io/managed"),
				Value: pulumi.String("true"),
			},
		},
		NodeConfigDetails: &containerengine.NodePoolNodeConfigDetailsArgs{
			Size: pulumi.Int(i.WorkerNodeInitialCount),
			PlacementConfigs: containerengine.NodePoolNodeConfigDetailsPlacementConfigArray{
				&containerengine.NodePoolNodeConfigDetailsPlacementConfigArgs{
					AvailabilityDomain: pulumi.String(availabilityDomain),
					SubnetId:           privateSubnet.ID(),
				},
			},
		},
		NodeSourceDetails: &containerengine.NodePoolNodeSourceDetailsArgs{
			ImageId:             pulumi.String(imageOCID),
			SourceType:          pulumi.String("IMAGE"),
			BootVolumeSizeInGbs: pulumi.String("50"),
		},
		NodeShapeConfig: &containerengine.NodePoolNodeShapeConfigArgs{
			Ocpus:       pulumi.Float64(2.0),
			MemoryInGbs: pulumi.Float64(12.0),
		},
	}, pulumi.Provider(ociProvider))
	if err != nil {
		return fmt.Errorf("failed to create node pool: %v", err)
	}

	// Export outputs
	ctx.Export("clusterID", cluster.ID())
	ctx.Export("nodePoolID", nodePool.ID())
	ctx.Export("clusterName", cluster.Name)
	ctx.Export("kubeConfig", pulumi.String("")) // Would need to generate actual kubeconfig

	return nil
}

// RunStage2Infrastructure executes the Stage 2 infrastructure deployment
func (i *OCIInfrastructurePulumi) RunStage2Infrastructure() (*InfrastructureOutputs, error) {
	ctx := context.Background()

	fmt.Printf("Running Stage 2 infrastructure deployment in target region: %s\n", i.TargetRegion)

	// Get private key path for service user
	// Set up Pulumi workspace using shared utility (no explicit credentials, using provider args instead)
	stack, err := SetupPulumiWorkspace(&PulumiWorkspaceConfig{
		ProjectName:   "infrastructure",
		InstanceName:  fmt.Sprintf("infrastructure-%s", i.RuntimeInstanceName),
		Program:       i.infrastructurePulumiProgram,
		Region:        i.TargetRegion,
		ConfigProfile: "THREEPORT_SERVICE",
		TenancyOCID:   i.TenancyOCID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to setup Pulumi workspace: %v", err)
	}

	// Run pulumi up
	upRes, err := stack.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("failed to run pulumi up: %v", err)
	}

	// Extract outputs
	outputs := &InfrastructureOutputs{
		ClusterID:   upRes.Outputs["clusterID"].Value.(string),
		NodePoolID:  upRes.Outputs["nodePoolID"].Value.(string),
		ClusterName: upRes.Outputs["clusterName"].Value.(string),
		KubeConfig:  upRes.Outputs["kubeConfig"].Value.(string),
	}

	return outputs, nil
}

// CreateWithTwoStagePulumi runs both bootstrap and infrastructure stages using Pulumi
func CreateOKEWithTwoStagePulumi(runtimeInstanceName, version, targetRegion, workerNodeShape string, workerNodeInitialCount int32) (*kube.KubeConnectionInfo, error) {
	fmt.Println("Starting two-stage Pulumi OCI deployment...")

	// Stage 1: Bootstrap (global resources in home region)
	bootstrap, err := NewOCIBootstrapPulumi(runtimeInstanceName, targetRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to create bootstrap instance: %w", err)
	}

	bootstrapOutputs, err := bootstrap.RunStage1Bootstrap()
	if err != nil {
		return nil, fmt.Errorf("stage 1 bootstrap failed: %w", err)
	}

	fmt.Printf("Stage 1 bootstrap completed. Compartment OCID: %s\n", bootstrapOutputs.CompartmentOCID)

	// Stage 2: Infrastructure (regional resources in target region)
	infrastructure := NewOCIInfrastructurePulumi(
		runtimeInstanceName,
		version,
		bootstrapOutputs.CompartmentOCID,
		bootstrap.TenancyOCID, // Pass the actual tenancy OCID
		targetRegion,
		workerNodeShape,
		workerNodeInitialCount,
		bootstrapOutputs,
	)

	infrastructureOutputs, err := infrastructure.RunStage2Infrastructure()
	if err != nil {
		return nil, fmt.Errorf("stage 2 infrastructure deployment failed: %w", err)
	}

	fmt.Printf("Stage 2 infrastructure deployment completed. Cluster ID: %s\n", infrastructureOutputs.ClusterID)

	// TODO: Generate and return actual KubeConnectionInfo from cluster
	return &kube.KubeConnectionInfo{
		APIEndpoint:     fmt.Sprintf("https://cluster-%s.us-ashburn-1.oraclecloud.com", infrastructureOutputs.ClusterID),
		CACertificate:   "", // Would need to extract from cluster
		Certificate:     "",
		Key:             "",
		Token:           "",
		TokenExpiration: time.Time{},
	}, nil
}

// DeleteOKEWithTwoStagePulumi tears down both infrastructure and bootstrap stacks
func DeleteOKEWithTwoStagePulumi(runtimeInstanceName string) error {
	fmt.Println("Starting two-stage Pulumi OCI teardown...")

	// Stage 1: Destroy infrastructure stack (regional resources)
	fmt.Println("Destroying Stage 2 infrastructure stack...")
	if err := destroyInfrastructureStack(runtimeInstanceName); err != nil {
		// Don't fail completely if infrastructure stack fails - continue to bootstrap cleanup
		fmt.Printf("Warning: failed to destroy infrastructure stack: %v\n", err)
		fmt.Println("Continuing with bootstrap cleanup...")
	} else {
		fmt.Println("Stage 2 infrastructure stack destroyed successfully")
	}

	// Stage 2: Destroy bootstrap stack (global resources) 
	fmt.Println("Destroying Stage 1 bootstrap stack...")
	if err := DeleteOCIBootstrapResources(runtimeInstanceName); err != nil {
		return fmt.Errorf("failed to destroy bootstrap stack: %w", err)
	}
	fmt.Println("Stage 1 bootstrap stack destroyed successfully")

	fmt.Println("Two-stage Pulumi OCI teardown completed")
	return nil
}

// destroyInfrastructureStack destroys the Stage 2 infrastructure Pulumi stack
func destroyInfrastructureStack(runtimeInstanceName string) error {
	// We need to determine the target region and other parameters
	// For now, we'll try to destroy based on the runtime instance name
	// This is a simplified approach - in a full implementation, we'd store this info
	
	// Try to get config from environment or use defaults
	targetRegion := "us-ashburn-1" // Default region - could be made configurable
	
	// Set up Pulumi workspace for infrastructure stack
	stack, err := SetupPulumiWorkspace(&PulumiWorkspaceConfig{
		ProjectName:   "infrastructure", 
		InstanceName:  fmt.Sprintf("infrastructure-%s", runtimeInstanceName),
		Program:       func(ctx *pulumi.Context) error { return nil }, // Empty program for destroy
		Region:        targetRegion,
		ConfigProfile: "THREEPORT_SERVICE",
		TenancyOCID:   "", // Not needed for destroy
	})
	if err != nil {
		return fmt.Errorf("failed to setup Pulumi workspace for infrastructure: %v", err)
	}

	// Destroy the infrastructure stack
	ctx := context.Background()
	_, err = stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	if err != nil {
		return fmt.Errorf("failed to destroy infrastructure stack: %v", err)
	}

	return nil
}
