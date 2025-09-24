package provider

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
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
	pulumiIdentity "github.com/pulumi/pulumi-oci/sdk/v2/go/oci/identity"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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

// InfraAPIKeyPair holds the API key pair information (for infrastructure deployment)
type InfraAPIKeyPair struct {
	PublicKeyPEM  string
	PrivateKeyPEM string
	Fingerprint   string
}

// generateInfraOCIAPIKeyPair generates a real RSA key pair for OCI API authentication (for infrastructure)
func generateInfraOCIAPIKeyPair() (*InfraAPIKeyPair, error) {
	// Generate RSA private key using same approach as auth package
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA private key: %v", err)
	}

	// Use existing utility to convert private key to PEM
	privateKeyPEM := auth.GetPrivateKeyPEMEncoding(privateKey)

	// Generate public key PEM
	publicKey := &privateKey.PublicKey
	publicKeyDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %v", err)
	}
	publicKeyPEM := auth.GetPEMEncoding(publicKeyDER, "PUBLIC KEY")

	// Generate OCI-style MD5 fingerprint
	fingerprint := generateInfraOCIFingerprint(publicKeyDER)

	return &InfraAPIKeyPair{
		PublicKeyPEM:  publicKeyPEM,
		PrivateKeyPEM: privateKeyPEM,
		Fingerprint:   fingerprint,
	}, nil
}

// generateInfraOCIFingerprint generates the MD5 fingerprint for OCI API key (for infrastructure)
func generateInfraOCIFingerprint(publicKeyDER []byte) string {
	// OCI expects MD5 hash of the DER-encoded public key
	hash := md5.Sum(publicKeyDER)
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x",
		hash[0], hash[1], hash[2], hash[3], hash[4], hash[5], hash[6], hash[7],
		hash[8], hash[9], hash[10], hash[11], hash[12], hash[13], hash[14], hash[15])
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
	// =============== BOOTSTRAP RESOURCES (using DEFAULT profile in home region) ===============

	// Create OCI provider for bootstrap resources using DEFAULT profile (home region)
	bootstrapProvider, err := oci.NewProvider(ctx, "oci-bootstrap-provider", &oci.ProviderArgs{
		Region:            pulumi.String("us-phoenix-1"), // Always use home region for bootstrap
		ConfigFileProfile: pulumi.String("DEFAULT"),      // Use DEFAULT profile
	})
	if err != nil {
		return fmt.Errorf("failed to create bootstrap OCI provider: %v", err)
	}

	// Create compartment
	compartment, err := pulumiIdentity.NewCompartment(ctx, "threeport-compartment", &pulumiIdentity.CompartmentArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport compartment for %s", i.RuntimeInstanceName)),
	}, pulumi.Provider(bootstrapProvider))
	if err != nil {
		return fmt.Errorf("failed to create compartment: %v", err)
	}

	// Create service user
	serviceUser, err := pulumiIdentity.NewUser(ctx, "threeport-service-user", &pulumiIdentity.UserArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-service-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport service user for %s", i.RuntimeInstanceName)),
		Email:         pulumi.String("threeport@example.com"),
	}, pulumi.Provider(bootstrapProvider))
	if err != nil {
		return fmt.Errorf("failed to create service user: %v", err)
	}

	// Check if we already have a key pair for this instance
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}
	privateKeyPath := filepath.Join(homeDir, ".oci", fmt.Sprintf("threeport-service-%s.pem", i.RuntimeInstanceName))
	var keyPair *APIKeyPair
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		// No existing key, generate a new one using bootstrap functions
		keyPair, err = generateOCIAPIKeyPair()
		if err != nil {
			return fmt.Errorf("failed to generate API key pair: %v", err)
		}
		fmt.Printf("ℹ️  Generated new API key pair\n")
	} else {
		// Existing key found, read it and generate the corresponding public key
		privateKeyPEM, err := os.ReadFile(privateKeyPath)
		if err != nil {
			return fmt.Errorf("failed to read existing private key: %v", err)
		}
		// Generate the public key and fingerprint from existing private key using bootstrap functions
		keyPair, err = getAPIKeyPairFromPrivateKey(string(privateKeyPEM))
		if err != nil {
			return fmt.Errorf("failed to generate public key from existing private key: %v", err)
		}
		fmt.Printf("ℹ️  Using existing private key: %s\n", privateKeyPath)
	}

	// Create API key for service user
	apiKey, err := pulumiIdentity.NewApiKey(ctx, "threeport-service-user-api-key", &pulumiIdentity.ApiKeyArgs{
		UserId:   serviceUser.ID(),
		KeyValue: pulumi.String(keyPair.PublicKeyPEM),
	}, pulumi.Provider(bootstrapProvider), pulumi.IgnoreChanges([]string{"keyValue"}), pulumi.DependsOn([]pulumi.Resource{serviceUser}))
	if err != nil {
		return fmt.Errorf("failed to create API key: %v", err)
	}

	// Validate API key is working before proceeding with infrastructure resources
	validatedUserOCID := apiKey.UserId.ApplyT(func(userOCID string) (string, error) {
		fmt.Printf("🔑 Validating infrastructure provider authentication across multiple services...\n")
		maxAttempts := 60
		waitSeconds := 5
		fmt.Printf("  → Attempting authentication validation (max %d attempts, %ds intervals)...\n", maxAttempts, waitSeconds)

		err := util.Retry(maxAttempts, waitSeconds, func() error {
			// Create configProvider with the resolved userOCID and private key content
			configProvider := common.NewRawConfigurationProvider(
				i.TenancyOCID,         // Tenancy OCID
				userOCID,              // Resolved user OCID from API key
				i.TargetRegion,        // Target region
				keyPair.Fingerprint,   // API key fingerprint
				keyPair.PrivateKeyPEM, // Private key content (not file path)
				nil,                   // No passphrase
			)

			// Test Identity Service
			fmt.Printf("    → Testing Identity service...\n")
			identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
			if err != nil {
				return fmt.Errorf("failed to create identity client: %v", err)
			}
			identityRequest := identity.ListCompartmentsRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = identityClient.ListCompartments(context.Background(), identityRequest)
			if err != nil {
				return fmt.Errorf("identity service authentication failed: %v", err)
			}

			// Test Core Service (VCN/Networking)
			fmt.Printf("    → Testing Core service (networking)...\n")
			coreClient, err := ociCore.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
			if err != nil {
				return fmt.Errorf("failed to create core client: %v", err)
			}

			// Test VCNs
			vcnRequest := ociCore.ListVcnsRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = coreClient.ListVcns(context.Background(), vcnRequest)
			if err != nil {
				return fmt.Errorf("core service VCN authentication failed: %v", err)
			}

			// Test Security Lists (this is where failures have been occurring)
			fmt.Printf("      → Testing Security Lists specifically...\n")
			seclistRequest := ociCore.ListSecurityListsRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = coreClient.ListSecurityLists(context.Background(), seclistRequest)
			if err != nil {
				return fmt.Errorf("core service Security Lists authentication failed: %v", err)
			}

			// Test Route Tables
			fmt.Printf("      → Testing Route Tables...\n")
			rtRequest := ociCore.ListRouteTablesRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = coreClient.ListRouteTables(context.Background(), rtRequest)
			if err != nil {
				return fmt.Errorf("core service Route Tables authentication failed: %v", err)
			}

			// Test Subnets
			fmt.Printf("      → Testing Subnets...\n")
			subnetRequest := ociCore.ListSubnetsRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = coreClient.ListSubnets(context.Background(), subnetRequest)
			if err != nil {
				return fmt.Errorf("core service Subnets authentication failed: %v", err)
			}

			// Test Internet Gateways
			fmt.Printf("      → Testing Internet Gateways...\n")
			igwRequest := ociCore.ListInternetGatewaysRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = coreClient.ListInternetGateways(context.Background(), igwRequest)
			if err != nil {
				return fmt.Errorf("core service Internet Gateways authentication failed: %v", err)
			}

			// Test NAT Gateways
			fmt.Printf("      → Testing NAT Gateways...\n")
			natRequest := ociCore.ListNatGatewaysRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = coreClient.ListNatGateways(context.Background(), natRequest)
			if err != nil {
				return fmt.Errorf("core service NAT Gateways authentication failed: %v", err)
			}

			// Test Container Engine Service
			fmt.Printf("    → Testing Container Engine service...\n")
			ceClient, err := ociContainerEngine.NewContainerEngineClientWithConfigurationProvider(configProvider)
			if err != nil {
				return fmt.Errorf("failed to create container engine client: %v", err)
			}
			ceRequest := ociContainerEngine.ListClustersRequest{
				CompartmentId: common.String(i.TenancyOCID),
				Limit:         common.Int(1),
			}
			_, err = ceClient.ListClusters(context.Background(), ceRequest)
			if err != nil {
				return fmt.Errorf("container engine service authentication failed: %v", err)
			}

			fmt.Printf("  → ✅ Authentication successful across all services - API key is ready\n")
			return nil
		})

		if err != nil {
			return "", fmt.Errorf("infrastructure provider authentication failed after %d attempts over %d minutes: %v",
				maxAttempts, (maxAttempts*waitSeconds)/60, err)
		}

		fmt.Printf("  → Adding 30-second buffer for final service propagation...\n")
		time.Sleep(30 * time.Second)
		// Return the validated user OCID on success
		return userOCID, nil
	}).(pulumi.StringOutput)

	// Create bootstrap group
	bootstrapGroup, err := pulumiIdentity.NewGroup(ctx, "threeport-bootstrap-group", &pulumiIdentity.GroupArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-bootstrap-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport bootstrap group for %s", i.RuntimeInstanceName)),
	}, pulumi.Provider(bootstrapProvider))
	if err != nil {
		return fmt.Errorf("failed to create bootstrap group: %v", err)
	}

	// Create operational group
	operationalGroup, err := pulumiIdentity.NewGroup(ctx, "threeport-operational-group", &pulumiIdentity.GroupArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-operational-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport operational group for %s", i.RuntimeInstanceName)),
	}, pulumi.Provider(bootstrapProvider))
	if err != nil {
		return fmt.Errorf("failed to create operational group: %v", err)
	}

	// Create dynamic group
	_, err = pulumiIdentity.NewDynamicGroup(ctx, "threeport-dynamic-group", &pulumiIdentity.DynamicGroupArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-dynamic-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport dynamic group for %s", i.RuntimeInstanceName)),
		MatchingRule:  pulumi.Sprintf("ALL {instance.compartment.id = '%s'}", compartment.ID()),
	}, pulumi.Provider(bootstrapProvider), pulumi.DependsOn([]pulumi.Resource{compartment}))
	if err != nil {
		return fmt.Errorf("failed to create dynamic group: %v", err)
	}

	// Add service user to groups
	_, err = pulumiIdentity.NewUserGroupMembership(ctx, "bootstrap-group-membership", &pulumiIdentity.UserGroupMembershipArgs{
		UserId:  serviceUser.ID(),
		GroupId: bootstrapGroup.ID(),
	}, pulumi.Provider(bootstrapProvider), pulumi.DependsOn([]pulumi.Resource{serviceUser, bootstrapGroup}))
	if err != nil {
		return fmt.Errorf("failed to add service user to bootstrap group: %v", err)
	}

	_, err = pulumiIdentity.NewUserGroupMembership(ctx, "operational-group-membership", &pulumiIdentity.UserGroupMembershipArgs{
		UserId:  serviceUser.ID(),
		GroupId: operationalGroup.ID(),
	}, pulumi.Provider(bootstrapProvider), pulumi.DependsOn([]pulumi.Resource{serviceUser, operationalGroup}))
	if err != nil {
		return fmt.Errorf("failed to add service user to operational group: %v", err)
	}

	// Create bootstrap policy in root compartment
	_, err = pulumiIdentity.NewPolicy(ctx, "threeport-bootstrap-policy", &pulumiIdentity.PolicyArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-bootstrap-policy-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport bootstrap policy for %s", i.RuntimeInstanceName)),
		Statements: pulumi.StringArray{
			pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage all-resources in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to inspect compartments in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage clusters in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage virtual-network-family in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage instance-family in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage volume-family in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage load-balancers in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to use vnics in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to use network-security-groups in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to use private-ips in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage public-ips in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage object-family in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage tag-namespaces in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to manage tag-defaults in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to use tag-namespaces in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			// pulumi.Sprintf("Allow group threeport-bootstrap-%s to use subnets in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
		},
	}, pulumi.Provider(bootstrapProvider), pulumi.DeleteBeforeReplace(true), pulumi.DependsOn([]pulumi.Resource{compartment}))
	if err != nil {
		return fmt.Errorf("failed to create bootstrap policy: %v", err)
	}

	// Create operational policy in root compartment
	_, err = pulumiIdentity.NewPolicy(ctx, "threeport-operational-policy", &pulumiIdentity.PolicyArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-operational-policy-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport operational policy for %s", i.RuntimeInstanceName)),
		Statements: pulumi.StringArray{
			pulumi.Sprintf("Allow group threeport-operational-%s to inspect compartments in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
		},
	}, pulumi.Provider(bootstrapProvider), pulumi.DeleteBeforeReplace(true), pulumi.DependsOn([]pulumi.Resource{compartment}))
	if err != nil {
		return fmt.Errorf("failed to create operational policy: %v", err)
	}

	// Create dynamic group policy in root compartment
	_, err = pulumiIdentity.NewPolicy(ctx, "threeport-dynamic-group-policy", &pulumiIdentity.PolicyArgs{
		CompartmentId: pulumi.String(i.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-dynamic-group-policy-%s", i.RuntimeInstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport dynamic group policy for %s", i.RuntimeInstanceName)),
		Statements: pulumi.StringArray{
			pulumi.Sprintf("Allow dynamic-group threeport-dynamic-%s to manage cluster-family in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
			pulumi.Sprintf("Allow dynamic-group threeport-dynamic-%s to manage instance-family in compartment threeport-%s", i.RuntimeInstanceName, i.RuntimeInstanceName),
		},
	}, pulumi.Provider(bootstrapProvider), pulumi.DeleteBeforeReplace(true), pulumi.DependsOn([]pulumi.Resource{compartment}))
	if err != nil {
		return fmt.Errorf("failed to create dynamic group policy: %v", err)
	}

	// Write private key to file for infrastructure provider to use
	err = os.WriteFile(privateKeyPath, []byte(keyPair.PrivateKeyPEM), 0600)
	if err != nil {
		return fmt.Errorf("failed to write private key file: %v", err)
	}

	// =============== INFRASTRUCTURE RESOURCES (using service user credentials in target
	// region) ===============

	// Create OCI provider for infrastructure resources using validated service user credentials
	// This ensures validation completes successfully before creating the provider
	infrastructureProvider, err := oci.NewProvider(ctx, "oci-infrastructure-provider", &oci.ProviderArgs{
		Region:         pulumi.String(i.TargetRegion),
		TenancyOcid:    pulumi.String(i.TenancyOCID),
		UserOcid:       validatedUserOCID, // Use validated output - ensures auth works
		Fingerprint:    pulumi.String(keyPair.Fingerprint),
		PrivateKeyPath: pulumi.String(privateKeyPath),
	}, pulumi.DependsOn([]pulumi.Resource{apiKey}))
	if err != nil {
		return fmt.Errorf("failed to create infrastructure OCI provider: %v", err)
	}

	// Create VCN
	vcn, err := core.NewVcn(ctx, fmt.Sprintf("%s-vcn", i.RuntimeInstanceName), &core.VcnArgs{
		CompartmentId: compartment.ID(),
		CidrBlocks:    pulumi.StringArray{pulumi.String("10.0.0.0/16")},
		DisplayName:   pulumi.String(fmt.Sprintf("%s-vcn", i.RuntimeInstanceName)),
		DnsLabel:      pulumi.String("threeportvcn"),
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create VCN: %v", err)
	}

	// Create Internet Gateway
	internetGateway, err := core.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", i.RuntimeInstanceName), &core.InternetGatewayArgs{
		CompartmentId: compartment.ID(),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-igw", i.RuntimeInstanceName)),
		Enabled:       pulumi.Bool(true),
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create internet gateway: %v", err)
	}

	// Create NAT Gateway
	natGateway, err := core.NewNatGateway(ctx, fmt.Sprintf("%s-natgw", i.RuntimeInstanceName), &core.NatGatewayArgs{
		CompartmentId: compartment.ID(),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-natgw", i.RuntimeInstanceName)),
		BlockTraffic:  pulumi.Bool(false),
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
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
		CompartmentId: compartment.ID(),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-servicegw", i.RuntimeInstanceName)),
		Services: core.ServiceGatewayServiceArray{
			&core.ServiceGatewayServiceArgs{
				ServiceId: pulumi.String(serviceID),
			},
		},
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create service gateway: %v", err)
	}

	// Create Public Route Table
	publicRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
		CompartmentId: compartment.ID(),
		VcnId:         vcn.ID(),
		DisplayName:   pulumi.String(fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName)),
		RouteRules: core.RouteTableRouteRuleArray{
			&core.RouteTableRouteRuleArgs{
				NetworkEntityId: internetGateway.ID(),
				Destination:     pulumi.String("0.0.0.0/0"),
				DestinationType: pulumi.String("CIDR_BLOCK"),
			},
		},
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create public route table: %v", err)
	}

	// Create Private Route Table
	privateRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-private-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
		CompartmentId: compartment.ID(),
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
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create private route table: %v", err)
	}

	// Create Security Lists
	workerSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-worker-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
		CompartmentId: compartment.ID(),
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
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create worker security list: %v", err)
	}

	loadBalancerSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-lb-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
		CompartmentId: compartment.ID(),
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
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create load balancer security list: %v", err)
	}

	// Create Subnets
	publicSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
		CompartmentId:           compartment.ID(),
		VcnId:                   vcn.ID(),
		CidrBlock:               pulumi.String("10.0.10.0/24"),
		DisplayName:             pulumi.String(fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName)),
		DnsLabel:                pulumi.String("publicsubnet"),
		ProhibitInternetIngress: pulumi.Bool(false),
		ProhibitPublicIpOnVnic:  pulumi.Bool(false),
		RouteTableId:            publicRouteTable.ID(),
		SecurityListIds:         pulumi.StringArray{workerSecList.ID()},
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create public subnet: %v", err)
	}

	privateSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
		CompartmentId:           compartment.ID(),
		VcnId:                   vcn.ID(),
		CidrBlock:               pulumi.String("10.0.20.0/24"),
		DisplayName:             pulumi.String(fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName)),
		DnsLabel:                pulumi.String("privatesubnet"),
		ProhibitInternetIngress: pulumi.Bool(true),
		ProhibitPublicIpOnVnic:  pulumi.Bool(true),
		RouteTableId:            privateRouteTable.ID(),
		SecurityListIds:         pulumi.StringArray{workerSecList.ID()},
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create private subnet: %v", err)
	}

	loadBalancerSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-lb-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
		CompartmentId:           compartment.ID(),
		VcnId:                   vcn.ID(),
		CidrBlock:               pulumi.String("10.0.30.0/24"),
		DisplayName:             pulumi.String(fmt.Sprintf("%s-lb-subnet", i.RuntimeInstanceName)),
		DnsLabel:                pulumi.String("lbsubnet"),
		ProhibitInternetIngress: pulumi.Bool(false),
		ProhibitPublicIpOnVnic:  pulumi.Bool(false),
		RouteTableId:            publicRouteTable.ID(),
		SecurityListIds:         pulumi.StringArray{loadBalancerSecList.ID()},
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider}))
	if err != nil {
		return fmt.Errorf("failed to create load balancer subnet: %v", err)
	}

	// Create OKE Cluster
	cluster, err := containerengine.NewCluster(ctx, i.RuntimeInstanceName, &containerengine.ClusterArgs{
		CompartmentId:     compartment.ID(),
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
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider, publicSubnet, loadBalancerSubnet}))
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
	_, err = containerengine.NewNodePool(ctx, fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName), &containerengine.NodePoolArgs{
		ClusterId:         cluster.ID(),
		CompartmentId:     compartment.ID(),
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
	}, pulumi.Provider(infrastructureProvider), pulumi.DependsOn([]pulumi.Resource{apiKey, infrastructureProvider, cluster}))
	if err != nil {
		return fmt.Errorf("failed to create node pool: %v", err)
	}

	// Export outputs
	// ctx.Export("clusterID", cluster.ID())
	// ctx.Export("compartmentID", compartment.ID())
	// ctx.Export("clusterName", cluster.Name)
	// ctx.Export("kubeConfig", pulumi.String("")) // Would need to generate actual kubeconfig

	return nil
}

// RunStage2Infrastructure executes the Stage 2 infrastructure deployment
func (i *OCIInfrastructurePulumi) RunStage2Infrastructure() (*InfrastructureOutputs, error) {
	ctx := context.Background()

	fmt.Printf("Running Stage 2 infrastructure deployment in target region: %s\n", i.TargetRegion)

	// Get private key path for service user
	// Set up Pulumi workspace using DEFAULT profile (providers are explicit in program)
	stack, err := SetupPulumiWorkspace(&PulumiWorkspaceConfig{
		ProjectName:   "infrastructure",
		InstanceName:  fmt.Sprintf("infrastructure-%s", i.RuntimeInstanceName),
		Program:       i.infrastructurePulumiProgram,
		Region:        i.TargetRegion,
		ConfigProfile: "DEFAULT",
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

func (i *OCIInfrastructurePulumi) validateOCICredentialsInProgram(ctx context.Context) error {
	fmt.Println("  → Testing THREEPORT_SERVICE profile credentials...")

	// Create OCI config provider using the THREEPORT_SERVICE profile
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}
	configPath := filepath.Join(homeDir, ".oci", "config")
	configProvider := common.CustomProfileConfigProvider(configPath, "THREEPORT_SERVICE")

	// Test 1: Get basic configuration info
	fmt.Println("  → Testing basic OCI configuration...")
	tenancyOCID, err := configProvider.TenancyOCID()
	if err != nil {
		return fmt.Errorf("failed to get tenancy OCID: %v", err)
	}

	userOCID, err := configProvider.UserOCID()
	if err != nil {
		return fmt.Errorf("failed to get user OCID: %v", err)
	}

	region, err := configProvider.Region()
	if err != nil {
		return fmt.Errorf("failed to get region: %v", err)
	}

	fmt.Printf("  → Tenancy: %s\n", tenancyOCID)
	fmt.Printf("  → User: %s\n", userOCID)
	fmt.Printf("  → Region: %s\n", region)

	// Test 2: Make a read-only Identity API call
	fmt.Println("  → Testing Identity API access...")
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return fmt.Errorf("failed to create identity client: %v", err)
	}

	// Get user details to validate credentials and show which user is being used
	getUserRequest := identity.GetUserRequest{UserId: &userOCID}
	getUserResponse, err := identityClient.GetUser(context.Background(), getUserRequest)
	if err != nil {
		return fmt.Errorf("failed to call GetUser API: %v", err)
	}

	fmt.Printf("  → ✅ Successfully authenticated as user: %s\n", *getUserResponse.User.Name)
	if getUserResponse.User.Email != nil {
		fmt.Printf("  → ✅ User email: %s\n", *getUserResponse.User.Email)
	}

	// Test 3: Verify we can access the target compartment
	if i.CompartmentOCID != "" {
		fmt.Printf("  → Testing access to target compartment: %s\n", i.CompartmentOCID)
		getCompartmentRequest := identity.GetCompartmentRequest{CompartmentId: &i.CompartmentOCID}
		getCompartmentResponse, err := identityClient.GetCompartment(context.Background(), getCompartmentRequest)
		if err != nil {
			return fmt.Errorf("failed to access target compartment %s: %v", i.CompartmentOCID, err)
		}
		fmt.Printf("  → ✅ Successfully accessed target compartment: %s\n", *getCompartmentResponse.Compartment.Name)
	}

	fmt.Println("  → 🎉 All credential validation tests passed!")
	return nil
}
