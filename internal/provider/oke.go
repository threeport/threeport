package provider

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	ocicontainerengine "github.com/oracle/oci-go-sdk/v65/containerengine"
	ocicore "github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci/containerengine"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci/core"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	"gopkg.in/ini.v1"
)

// KubernetesRuntimeInfraOKE represents the infrastructure for a threeport-managed OKE
// (Oracle Kubernetes Engine) cluster.
type KubernetesRuntimeInfraOKE struct {
	// The unique name of the kubernetes runtime instance managed by threeport.
	RuntimeInstanceName string

	// The Oracle Cloud tenancy ID where the cluster infra is provisioned.
	TenancyID string

	// The Oracle Cloud compartment ID where resources will be created.
	CompartmentID string

	// The Oracle Cloud region where resources will be created.
	Region string

	// The number of availability domains the OKE cluster will be deployed across.
	AvailabilityDomainCount int32

	// The Oracle Cloud shape used for the worker nodes.
	WorkerNodeShape string

	// The number of nodes initially created for the worker node pool.
	WorkerNodeInitialCount int32

	// The minimum number of nodes to maintain in the worker node pool.
	WorkerNodeMinCount int32

	// The maximum number of nodes allowed in the worker node pool.
	WorkerNodeMaxCount int32

	// The path to the Pulumi state directory
	stateDir string
}

// loadOCIConfig reads the OCI configuration using the OCI SDK and updates the struct fields
func (i *KubernetesRuntimeInfraOKE) loadOCIConfig() error {
	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Path to OCI config file
	ociConfigPath := filepath.Join(homeDir, ".oci", "config")
	fmt.Printf("Loading OCI config from: %s\n", ociConfigPath)

	// Check if config file exists
	if _, err := os.Stat(ociConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("OCI config file not found at %s", ociConfigPath)
	}

	// Load the configuration using the OCI SDK
	configProvider, err := common.ConfigurationProviderFromFile(ociConfigPath, "")
	if err != nil {
		return fmt.Errorf("failed to load OCI configuration: %w", err)
	}

	// Get the tenancy OCID
	tenancyOCID, err := configProvider.TenancyOCID()
	if err != nil {
		return fmt.Errorf("failed to get tenancy OCID: %w", err)
	}
	fmt.Printf("Loaded tenancy OCID: %s\n", tenancyOCID)

	// Get the user OCID
	userOCID, err := configProvider.UserOCID()
	if err != nil {
		return fmt.Errorf("failed to get user OCID: %w", err)
	}
	fmt.Printf("Loaded user OCID: %s\n", userOCID)

	// Get the region
	region, err := configProvider.Region()
	if err != nil {
		return fmt.Errorf("failed to get region: %w", err)
	}
	fmt.Printf("Loaded region: %s\n", region)

	// Get the fingerprint
	fingerprint, err := configProvider.KeyFingerprint()
	if err != nil {
		return fmt.Errorf("failed to get key fingerprint: %w", err)
	}
	fmt.Printf("Loaded key fingerprint: %s\n", fingerprint)

	// Get the private key
	privateKey, err := configProvider.PrivateRSAKey()
	if err != nil {
		return fmt.Errorf("failed to get private key: %w", err)
	}

	// Convert private key to PEM-encoded string
	privateKeyPEM := privateKeyToPEM(privateKey)
	fmt.Printf("Successfully loaded private key\n")

	// Read the config file to get the compartment OCID
	cfg, err := ini.Load(ociConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read OCI config file: %w", err)
	}

	// Get the compartment OCID from the DEFAULT section
	compartmentOCID := cfg.Section("DEFAULT").Key("compartment_id").String()
	if compartmentOCID == "" {
		// If no compartment_id is specified, use the tenancy OCID as the root compartment
		compartmentOCID = tenancyOCID
	}
	fmt.Printf("Using compartment OCID: %s\n", compartmentOCID)

	// Update struct fields with values from config
	if i.TenancyID == "" {
		i.TenancyID = tenancyOCID
	}
	if i.CompartmentID == "" {
		i.CompartmentID = compartmentOCID
	}
	if i.Region == "" {
		i.Region = region
	}

	// Set environment variables for OCI authentication
	os.Setenv("OCI_TENANCY_OCID", tenancyOCID)
	os.Setenv("OCI_USER_OCID", userOCID)
	os.Setenv("OCI_REGION", region)
	os.Setenv("OCI_KEY_FINGERPRINT", fingerprint)
	os.Setenv("OCI_PRIVATE_KEY", privateKeyPEM)
	os.Setenv("OCI_COMPARTMENT_OCID", compartmentOCID)

	// Validate required fields
	if i.TenancyID == "" {
		return fmt.Errorf("tenancy ID not found in OCI config")
	}
	if i.CompartmentID == "" {
		return fmt.Errorf("compartment ID not found in OCI config")
	}
	if i.Region == "" {
		return fmt.Errorf("region not found in OCI config")
	}

	return nil
}

// privateKeyToPEM converts an RSA private key to a PEM-encoded string
func privateKeyToPEM(privateKey *rsa.PrivateKey) string {
	// Marshal the private key to PKCS#1 format
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)

	// Create a PEM block
	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privateKeyBytes,
		},
	)

	// Convert to string
	return string(privateKeyPEM)
}

// createDNSLabel creates a valid DNS label that meets OCI requirements:
// - Must be 15 characters or less
// - Must contain only lowercase alphanumeric characters
// - Maintains uniqueness by using parts of the original name
func createDNSLabel(name string) string {
	// Convert to lowercase
	dnsLabel := strings.ToLower(name)

	// If longer than 15 chars, take first 7 and last 7 with 'x' in middle
	if len(dnsLabel) > 15 {
		dnsLabel = dnsLabel[:7] + "x" + dnsLabel[len(dnsLabel)-7:]
	}

	// Replace any non-alphanumeric chars with 'x'
	dnsLabel = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return 'x'
	}, dnsLabel)

	return dnsLabel
}

// getAvailabilityDomainName returns the full name of the first availability domain in the region
func (i *KubernetesRuntimeInfraOKE) getAvailabilityDomainName() (string, error) {
	// Create a new identity client
	configProvider := common.DefaultConfigProvider()
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create identity client: %w", err)
	}

	// Set the region for the client
	identityClient.SetRegion(i.Region)

	// Create a request to list availability domains
	request := identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(i.CompartmentID),
	}

	// Call the API to get availability domains
	response, err := identityClient.ListAvailabilityDomains(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to list availability domains: %w", err)
	}

	// Check if we have any availability domains
	if len(response.Items) == 0 {
		return "", fmt.Errorf("no availability domains found in region %s", i.Region)
	}

	// Return the name of the first availability domain
	return *response.Items[0].Name, nil
}

// getServiceGatewayServiceID returns the OCI service ID for the service gateway in a given region.
// This ID is used to identify the Oracle Services Network in the service gateway.
func getServiceGatewayServiceID(region string, compartmentID string) (string, error) {
	// Create a new virtual network client
	configProvider := common.DefaultConfigProvider()
	vcnClient, err := ocicore.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create virtual network client: %w", err)
	}

	// Set the region for the client
	vcnClient.SetRegion(region)

	// Create a request to list services
	request := ocicore.ListServicesRequest{}

	// Call the API to get services
	response, err := vcnClient.ListServices(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to list services: %w", err)
	}

	// Find the Oracle Services Network service
	for _, service := range response.Items {
		if service.Name != nil && strings.Contains(*service.Name, "Services In Oracle Services Network") {
			return *service.Id, nil
		}
	}

	// If service not found, return an error
	return "", fmt.Errorf("Oracle Services Network service not found in region %s", region)
}

// getLatestOKEVersion returns the latest Kubernetes version available in OKE
func (i *KubernetesRuntimeInfraOKE) getLatestOKEVersion() (string, error) {
	// Create a new container engine client
	configProvider := common.DefaultConfigProvider()
	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create container engine client: %w", err)
	}

	// Set the region for the client
	containerClient.SetRegion(i.Region)

	// Create a request to list node pool options
	request := ocicontainerengine.GetNodePoolOptionsRequest{
		CompartmentId:    common.String(i.CompartmentID),
		NodePoolOptionId: common.String("all"),
	}

	// Call the API to get node pool options
	response, err := containerClient.GetNodePoolOptions(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to get node pool options: %w", err)
	}

	// Check if we have any images
	if len(response.Sources) == 0 {
		return "", fmt.Errorf("no OKE worker node images found")
	}

	// Find the latest version by parsing version strings
	latestVersion := ""
	for _, source := range response.Sources {
		if sourceType, ok := source.(ocicontainerengine.NodeSourceViaImageOption); ok {
			name := *sourceType.SourceName
			// Extract version from name (e.g., "OKE-1.30.10")
			if strings.Contains(name, "OKE-") {
				version := strings.Split(name, "OKE-")[1]
				version = strings.Split(version, "-")[0] // Remove any trailing parts
				if latestVersion == "" || version > latestVersion {
					latestVersion = version
				}
			}
		}
	}

	if latestVersion == "" {
		return "", fmt.Errorf("could not determine latest OKE version")
	}

	latestVersion = "v" + latestVersion

	return latestVersion, nil
}

// getOKEWorkerNodeImageOCID returns the OCID of the specified OKE worker node image
func (i *KubernetesRuntimeInfraOKE) getOKEWorkerNodeImageOCID() (string, error) {
	// Get the latest OKE version
	// latestVersion, err := i.getLatestOKEVersion()
	// if err != nil {
	// 	return "", fmt.Errorf("failed to get latest OKE version: %w", err)
	// }
	latestVersion := "1.30.10"

	// Create a new container engine client
	configProvider := common.DefaultConfigProvider()
	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create container engine client: %w", err)
	}

	// Set the region for the client
	containerClient.SetRegion(i.Region)

	// Create a request to list node pool options
	request := ocicontainerengine.GetNodePoolOptionsRequest{
		CompartmentId:    common.String(i.CompartmentID),
		NodePoolOptionId: common.String("all"),
	}

	// Call the API to get node pool options
	response, err := containerClient.GetNodePoolOptions(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to get node pool options: %w", err)
	}

	// Check if we have any images
	if len(response.Sources) == 0 {
		return "", fmt.Errorf("no OKE worker node images found")
	}

	// Print out all available images
	fmt.Println("\nAvailable OKE worker node images:")
	for _, source := range response.Sources {
		// Try to get the concrete type
		if sourceType, ok := source.(ocicontainerengine.NodeSourceViaImageOption); ok {
			fmt.Printf("- Name: %s, OCID: %s\n", *sourceType.SourceName, *sourceType.ImageId)
		}
	}

	// Find an image with the latest Kubernetes version
	for _, source := range response.Sources {
		// Try to get the concrete type
		if sourceType, ok := source.(ocicontainerengine.NodeSourceViaImageOption); ok {
			name := *sourceType.SourceName
			if strings.Contains(name, fmt.Sprintf("OKE-%s", latestVersion)) && strings.Contains(name, "aarch64") {
				return *sourceType.ImageId, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable OKE worker node images found with Kubernetes version %s and aarch64 architecture", latestVersion)
}

// Create installs a Kubernetes cluster using Oracle Cloud OKE for threeport workloads.
func (i *KubernetesRuntimeInfraOKE) Create() (*kube.KubeConnectionInfo, error) {
	// Load OCI configuration
	if err := i.loadOCIConfig(); err != nil {
		return nil, fmt.Errorf("failed to load OCI configuration: %w", err)
	}

	// Get the latest OKE version
	latestVersion, err := i.getLatestOKEVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest OKE version: %w", err)
	}
	// latestVersion := "v1.30.10"

	// Set default values for worker nodes if not specified
	if i.WorkerNodeShape == "" {
		i.WorkerNodeShape = "VM.Standard.A1.Flex"
	}
	if i.WorkerNodeInitialCount == 0 {
		i.WorkerNodeInitialCount = 2
	}
	if i.WorkerNodeMinCount == 0 {
		i.WorkerNodeMinCount = 2
	}
	if i.WorkerNodeMaxCount == 0 {
		i.WorkerNodeMaxCount = 2
	}

	// Get the availability domain name
	availabilityDomain, err := i.getAvailabilityDomainName()
	if err != nil {
		return nil, fmt.Errorf("failed to get availability domain: %w", err)
	}

	// Set up state directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	i.stateDir = filepath.Join(homeDir, ".config", "threeport", "pulumi-state", i.RuntimeInstanceName)

	// Ensure state directory exists
	if err := os.MkdirAll(i.stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	// Create Pulumi.yaml project file
	pulumiYaml := `name: oke
runtime: go
description: Oracle Kubernetes Engine (OKE) cluster for Threeport
`
	pulumiYamlPath := filepath.Join(i.stateDir, "Pulumi.yaml")
	if err := os.WriteFile(pulumiYamlPath, []byte(pulumiYaml), 0644); err != nil {
		return nil, fmt.Errorf("failed to create Pulumi.yaml: %w", err)
	}

	// Create a new Pulumi program
	program := func(ctx *pulumi.Context) error {
		// Create OCI provider with explicit configuration
		ociProvider, err := oci.NewProvider(ctx, "oci-provider", &oci.ProviderArgs{
			Region:      pulumi.String(i.Region),
			TenancyOcid: pulumi.String(i.TenancyID),
		})
		if err != nil {
			return fmt.Errorf("failed to create OCI provider: %w", err)
		}

		// Create VCN for the cluster
		vcn, err := core.NewVcn(ctx, fmt.Sprintf("%s-vcn", i.RuntimeInstanceName), &core.VcnArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			CidrBlock:     pulumi.String("10.0.0.0/16"),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-vcn", i.RuntimeInstanceName)),
			DnsLabel:      pulumi.String(createDNSLabel(i.RuntimeInstanceName)),
		}, pulumi.Provider(ociProvider),
			pulumi.DeleteBeforeReplace(true),
			pulumi.Protect(false))
		if err != nil {
			return fmt.Errorf("failed to create VCN: %w", err)
		}

		// Create Internet Gateway
		internetGateway, err := core.NewInternetGateway(ctx, fmt.Sprintf("%s-ig", i.RuntimeInstanceName), &core.InternetGatewayArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-ig", i.RuntimeInstanceName)),
			Enabled:       pulumi.Bool(true),
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create Internet Gateway: %w", err)
		}

		// Create NAT Gateway
		natGateway, err := core.NewNatGateway(ctx, fmt.Sprintf("%s-ng", i.RuntimeInstanceName), &core.NatGatewayArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-ng", i.RuntimeInstanceName)),
			BlockTraffic:  pulumi.Bool(false),
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create NAT Gateway: %w", err)
		}

		// Get the service gateway service ID
		serviceID, err := getServiceGatewayServiceID(i.Region, i.CompartmentID)
		if err != nil {
			return fmt.Errorf("failed to get service gateway service ID: %w", err)
		}

		// Create Service Gateway
		serviceGateway, err := core.NewServiceGateway(ctx, fmt.Sprintf("%s-sg", i.RuntimeInstanceName), &core.ServiceGatewayArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-sg", i.RuntimeInstanceName)),
			Services: core.ServiceGatewayServiceArray{
				&core.ServiceGatewayServiceArgs{
					ServiceId: pulumi.String(serviceID),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create Service Gateway: %w", err)
		}

		// Create route table for public subnet
		publicRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName)),
			RouteRules: core.RouteTableRouteRuleArray{
				&core.RouteTableRouteRuleArgs{
					Destination:     pulumi.String("0.0.0.0/0"),
					DestinationType: pulumi.String("CIDR_BLOCK"),
					NetworkEntityId: internetGateway.ID(),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{internetGateway}))
		if err != nil {
			return fmt.Errorf("failed to create public route table: %w", err)
		}

		// Create route table for private subnet
		privateRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-private-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-private-rt", i.RuntimeInstanceName)),
			RouteRules: core.RouteTableRouteRuleArray{
				&core.RouteTableRouteRuleArgs{
					Destination:     pulumi.String("0.0.0.0/0"),
					DestinationType: pulumi.String("CIDR_BLOCK"),
					NetworkEntityId: natGateway.ID(),
				},
				&core.RouteTableRouteRuleArgs{
					Destination:     pulumi.String("all-phx-services-in-oracle-services-network"),
					DestinationType: pulumi.String("SERVICE_CIDR_BLOCK"),
					NetworkEntityId: serviceGateway.ID(),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{natGateway, serviceGateway}))
		if err != nil {
			return fmt.Errorf("failed to create private route table: %w", err)
		}

		// Create public subnet for load balancers
		publicSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
			AvailabilityDomain:     pulumi.String(availabilityDomain),
			CidrBlock:              pulumi.String("10.0.1.0/24"),
			CompartmentId:          pulumi.String(i.CompartmentID),
			VcnId:                  vcn.ID(),
			DisplayName:            pulumi.String(fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName)),
			DnsLabel:               pulumi.String(createDNSLabel(fmt.Sprintf("%s-public", i.RuntimeInstanceName))),
			ProhibitPublicIpOnVnic: pulumi.Bool(false),
			RouteTableId:           publicRouteTable.ID(),
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn, publicRouteTable}))
		if err != nil {
			return fmt.Errorf("failed to create public subnet: %w", err)
		}

		// Create private subnet for worker nodes
		privateSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
			AvailabilityDomain:     pulumi.String(availabilityDomain),
			CidrBlock:              pulumi.String("10.0.2.0/24"),
			CompartmentId:          pulumi.String(i.CompartmentID),
			VcnId:                  vcn.ID(),
			DisplayName:            pulumi.String(fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName)),
			DnsLabel:               pulumi.String(createDNSLabel(fmt.Sprintf("%s-private", i.RuntimeInstanceName))),
			ProhibitPublicIpOnVnic: pulumi.Bool(true),
			RouteTableId:           privateRouteTable.ID(),
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn, privateRouteTable}))
		if err != nil {
			return fmt.Errorf("failed to create private subnet: %w", err)
		}

		// Create OKE Cluster with explicit dependency on networking components
		cluster, err := containerengine.NewCluster(ctx, i.RuntimeInstanceName, &containerengine.ClusterArgs{
			CompartmentId:     pulumi.String(i.CompartmentID),
			Name:              pulumi.String(i.RuntimeInstanceName),
			VcnId:             vcn.ID(),
			KubernetesVersion: pulumi.String(latestVersion),
			Options: &containerengine.ClusterOptionsArgs{
				KubernetesNetworkConfig: &containerengine.ClusterOptionsKubernetesNetworkConfigArgs{
					PodsCidr:     pulumi.String("10.244.0.0/16"),
					ServicesCidr: pulumi.String("10.96.0.0/12"),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{
				vcn,
				internetGateway,
				natGateway,
				serviceGateway,
				publicSubnet,
				privateSubnet,
				publicRouteTable,
				privateRouteTable,
			}))
		if err != nil {
			return fmt.Errorf("failed to create OKE cluster: %w", err)
		}

		// Get the OKE worker node image OCID
		imageOCID, err := i.getOKEWorkerNodeImageOCID()
		if err != nil {
			return fmt.Errorf("failed to get OKE worker node image OCID: %w", err)
		}
		fmt.Printf("Using OKE worker node image OCID: %s\n", imageOCID)

		// Create Node Pool with explicit dependency on cluster
		fmt.Printf("Creating node pool with shape: %s, initial count: %d\n", i.WorkerNodeShape, i.WorkerNodeInitialCount)
		_, err = containerengine.NewNodePool(ctx, fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName), &containerengine.NodePoolArgs{
			ClusterId:         cluster.ID(),
			CompartmentId:     pulumi.String(i.CompartmentID),
			Name:              pulumi.String(fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName)),
			NodeShape:         pulumi.String(i.WorkerNodeShape),
			KubernetesVersion: pulumi.String(latestVersion),
			InitialNodeLabels: containerengine.NodePoolInitialNodeLabelArray{
				&containerengine.NodePoolInitialNodeLabelArgs{
					Key:   pulumi.String("threeport.io/managed"),
					Value: pulumi.String("true"),
				},
			},
			NodeConfigDetails: &containerengine.NodePoolNodeConfigDetailsArgs{
				Size: pulumi.Int(int(i.WorkerNodeInitialCount)),
				PlacementConfigs: containerengine.NodePoolNodeConfigDetailsPlacementConfigArray{
					&containerengine.NodePoolNodeConfigDetailsPlacementConfigArgs{
						AvailabilityDomain: pulumi.String(availabilityDomain),
						SubnetId:           privateSubnet.ID(),
					},
				},
			},
			NodeSourceDetails: &containerengine.NodePoolNodeSourceDetailsArgs{
				ImageId:    pulumi.String(imageOCID),
				SourceType: pulumi.String("IMAGE"),
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			fmt.Printf("Failed to create node pool: %v\n", err)
			return fmt.Errorf("failed to create node pool: %w", err)
		}

		// Export cluster ID and kubeconfig for later use
		ctx.Export("clusterId", cluster.ID())
		ctx.Export("kubeconfig", cluster.Endpoints.Index(pulumi.Int(0)).PrivateEndpoint())

		return nil
	}

	// Create a context for the automation API
	ctx := context.Background()

	// Set environment variables for Pulumi configuration
	os.Setenv("PULUMI_BACKEND_URL", "file://"+i.stateDir)
	os.Setenv("PULUMI_HOME", i.stateDir)
	os.Setenv("PULUMI_ORGANIZATION", "organization")
	os.Setenv("PULUMI_PROJECT", "oke")
	os.Setenv("PULUMI_CONFIG_PASSPHRASE", "threeport") // Set a default passphrase for state encryption

	// Set plugin path to the default location
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	defaultPluginPath := filepath.Join(userHomeDir, ".pulumi", "plugins")
	os.Setenv("PULUMI_PLUGIN_PATH", defaultPluginPath)

	// Create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(ctx,
		auto.Program(program),
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Create or select a stack with fully qualified name
	stackName := fmt.Sprintf("organization/oke/%s", i.RuntimeInstanceName)
	stack, err := auto.UpsertStack(ctx, stackName, workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to create/select stack: %w", err)
	}

	// Set up stack configuration
	err = stack.SetConfig(ctx, "oci:region", auto.ConfigValue{Value: i.Region})
	if err != nil {
		return nil, fmt.Errorf("failed to set region config: %w", err)
	}

	// Set OCI environment variables
	os.Setenv("OCI_REGION", i.Region)
	os.Setenv("OCI_TENANCY_OCID", i.TenancyID)
	os.Setenv("OCI_COMPARTMENT_OCID", i.CompartmentID)

	// Deploy the stack
	_, err = stack.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("failed to deploy stack: %w", err)
	}

	// Get the stack outputs
	outputs, err := stack.Outputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stack outputs: %w", err)
	}

	// Extract cluster ID and kubeconfig from outputs
	clusterIDValue, ok := outputs["clusterId"]
	if !ok {
		return nil, fmt.Errorf("failed to get cluster ID from outputs")
	}
	clusterID, ok := clusterIDValue.Value.(string)
	if !ok {
		return nil, fmt.Errorf("cluster ID output is not a string")
	}

	kubeconfigValue, ok := outputs["kubeconfig"]
	if !ok {
		return nil, fmt.Errorf("failed to get kubeconfig from outputs")
	}
	kubeconfig, ok := kubeconfigValue.Value.(string)
	if !ok {
		return nil, fmt.Errorf("kubeconfig output is not a string")
	}

	// Return the connection info
	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:   clusterID,
		CACertificate: kubeconfig,
	}

	return kubeConnInfo, nil
}

// Delete deletes an Oracle Cloud OKE cluster.
func (i *KubernetesRuntimeInfraOKE) Delete() error {
	if i.stateDir == "" {
		return fmt.Errorf("state directory not initialized")
	}

	// Check if state directory exists
	if _, err := os.Stat(i.stateDir); os.IsNotExist(err) {
		return fmt.Errorf("state directory does not exist: %s", i.stateDir)
	}

	// Set environment variable for Pulumi state directory
	os.Setenv("PULUMI_HOME", i.stateDir)

	// Set up Pulumi project and stack
	os.Setenv("PULUMI_PROJECT", i.RuntimeInstanceName)
	os.Setenv("PULUMI_STACK", i.RuntimeInstanceName)
	os.Setenv("PULUMI_MONITOR_ADDRESS", "127.0.0.1:60005")

	// Create a program that will destroy resources
	program := func(ctx *pulumi.Context) error {
		// The program will read the existing state and destroy resources
		return nil
	}

	// Execute the program with destroy flag
	os.Setenv("PULUMI_DESTROY", "true")
	if err := pulumi.RunErr(program); err != nil {
		return fmt.Errorf("failed to destroy Pulumi resources: %w", err)
	}

	// Remove the state directory after successful destruction
	if err := os.RemoveAll(i.stateDir); err != nil {
		return fmt.Errorf("failed to remove state directory: %w", err)
	}

	return nil
}

// GetConnection gets the latest connection info for authentication to an OKE cluster.
func (i *KubernetesRuntimeInfraOKE) GetConnection() (*kube.KubeConnectionInfo, error) {
	if i.stateDir == "" {
		return nil, fmt.Errorf("state directory not initialized")
	}

	// Check if state directory exists
	if _, err := os.Stat(i.stateDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("state directory does not exist: %s", i.stateDir)
	}

	// For now, return placeholder values
	// In a real implementation, you would parse the Pulumi state file to get the actual values
	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:   "placeholder-cluster-id",
		CACertificate: "placeholder-kubeconfig",
	}

	return kubeConnInfo, nil
}

// OKEInventoryFilepath returns a standardized filename and path for the OKE
// inventory file.
func OKEInventoryFilepath(providerConfigDir, instanceName string) string {
	inventoryFilename := fmt.Sprintf("oke-inventory-%s.json", instanceName)
	return filepath.Join(providerConfigDir, inventoryFilename)
}
