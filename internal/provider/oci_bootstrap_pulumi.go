package provider

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
	pulumiIdentity "github.com/pulumi/pulumi-oci/sdk/v2/go/oci/identity"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// OCIBootstrapPulumi handles the two-stage Pulumi bootstrap process for OCI resources
type OCIBootstrapPulumi struct {
	TenancyOCID          string
	HomeRegion           string // User's home region for global operations
	TargetRegion         string // Target region for infrastructure deployment
	InstanceName         string
	CompartmentName      string
	ServiceUserName      string
	ServiceUserEmail     string
	BootstrapGroupName   string
	OperationalGroupName string
	DynamicGroupName     string
	configProvider       common.ConfigurationProvider
}

// BootstrapOutputs represents the outputs from Stage 1 bootstrap stack
type BootstrapOutputs struct {
	CompartmentOCID        string
	ServiceUserOCID        string
	ServiceUserAPIKey      string
	ServiceUserPrivateKey  string
	ServiceUserFingerprint string
	BootstrapGroupOCID     string
	OperationalGroupOCID   string
	DynamicGroupOCID       string
	BootstrapPolicyOCID    string
	OperationalPolicyOCID  string
	DynamicGroupPolicyOCID string
}

// APIKeyPair represents a generated RSA key pair
type APIKeyPair struct {
	PublicKeyPEM  string
	PrivateKeyPEM string
	Fingerprint   string
}

// NewOCIBootstrapPulumi creates a new OCIBootstrapPulumi instance
func NewOCIBootstrapPulumi(instanceName, targetRegion string) (*OCIBootstrapPulumi, error) {
	configProvider := common.DefaultConfigProvider()

	// Get home region from DEFAULT profile
	homeRegion, err := configProvider.Region()
	if err != nil {
		return nil, fmt.Errorf("failed to get home region from DEFAULT profile: %v", err)
	}

	tenancyOCID, err := configProvider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("failed to get tenancy OCID: %v", err)
	}

	return &OCIBootstrapPulumi{
		TenancyOCID:          tenancyOCID,
		HomeRegion:           homeRegion,
		TargetRegion:         targetRegion,
		InstanceName:         instanceName,
		CompartmentName:      fmt.Sprintf("threeport-%s", instanceName),
		ServiceUserName:      fmt.Sprintf("threeport-service-%s", instanceName),
		ServiceUserEmail:     fmt.Sprintf("threeport-service-%s@example.com", instanceName),
		BootstrapGroupName:   fmt.Sprintf("threeport-bootstrap-%s", instanceName),
		OperationalGroupName: fmt.Sprintf("threeport-operational-%s", instanceName),
		DynamicGroupName:     fmt.Sprintf("threeport-dynamic-%s", instanceName),
		configProvider:       configProvider,
	}, nil
}

// bootstrapPulumiProgram defines the Pulumi program for Stage 1 bootstrap
func (b *OCIBootstrapPulumi) bootstrapPulumiProgram(ctx *pulumi.Context) error {
	// Create compartment
	compartment, err := pulumiIdentity.NewCompartment(ctx, "threeport-compartment", &pulumiIdentity.CompartmentArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(b.CompartmentName),
		Description:   pulumi.String(fmt.Sprintf("Threeport compartment for %s", b.InstanceName)),
	})
	if err != nil {
		return fmt.Errorf("failed to create compartment: %v", err)
	}

	// Create service user
	serviceUser, err := pulumiIdentity.NewUser(ctx, "threeport-service-user", &pulumiIdentity.UserArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(b.ServiceUserName),
		Description:   pulumi.String(fmt.Sprintf("Threeport service user for %s", b.InstanceName)),
		Email:         pulumi.String(b.ServiceUserEmail),
	})
	if err != nil {
		return fmt.Errorf("failed to create service user: %v", err)
	}

	// Check if we already have a key pair for this instance
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}
	privateKeyPath := filepath.Join(homeDir, ".oci", fmt.Sprintf("threeport-service-%s.pem", b.InstanceName))

	var keyPair *APIKeyPair
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		// No existing key, generate a new one
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

		// Generate the public key and fingerprint from existing private key
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
	}, pulumi.IgnoreChanges([]string{"keyValue"}))
	if err != nil {
		return fmt.Errorf("failed to create API key: %v", err)
	}

	// Create bootstrap group
	bootstrapGroup, err := pulumiIdentity.NewGroup(ctx, "threeport-bootstrap-group", &pulumiIdentity.GroupArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(b.BootstrapGroupName),
		Description:   pulumi.String(fmt.Sprintf("Threeport bootstrap group for %s", b.InstanceName)),
	})
	if err != nil {
		return fmt.Errorf("failed to create bootstrap group: %v", err)
	}

	// Create operational group
	operationalGroup, err := pulumiIdentity.NewGroup(ctx, "threeport-operational-group", &pulumiIdentity.GroupArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(b.OperationalGroupName),
		Description:   pulumi.String(fmt.Sprintf("Threeport operational group for %s", b.InstanceName)),
	})
	if err != nil {
		return fmt.Errorf("failed to create operational group: %v", err)
	}

	// Create dynamic group
	dynamicGroup, err := pulumiIdentity.NewDynamicGroup(ctx, "threeport-dynamic-group", &pulumiIdentity.DynamicGroupArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(b.DynamicGroupName),
		Description:   pulumi.String(fmt.Sprintf("Threeport dynamic group for %s", b.InstanceName)),
		MatchingRule:  pulumi.Sprintf("ALL {instance.compartment.id = '%s'}", compartment.ID()),
	})
	if err != nil {
		return fmt.Errorf("failed to create dynamic group: %v", err)
	}

	// Add service user to groups
	_, err = pulumiIdentity.NewUserGroupMembership(ctx, "bootstrap-group-membership", &pulumiIdentity.UserGroupMembershipArgs{
		UserId:  serviceUser.ID(),
		GroupId: bootstrapGroup.ID(),
	})
	if err != nil {
		return fmt.Errorf("failed to add service user to bootstrap group: %v", err)
	}

	_, err = pulumiIdentity.NewUserGroupMembership(ctx, "operational-group-membership", &pulumiIdentity.UserGroupMembershipArgs{
		UserId:  serviceUser.ID(),
		GroupId: operationalGroup.ID(),
	})
	if err != nil {
		return fmt.Errorf("failed to add service user to operational group: %v", err)
	}

	// Create bootstrap policy in root compartment
	bootstrapPolicy, err := pulumiIdentity.NewPolicy(ctx, "threeport-bootstrap-policy", &pulumiIdentity.PolicyArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-bootstrap-policy-%s", b.InstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport bootstrap policy for %s", b.InstanceName)),
		Statements: pulumi.StringArray{
			pulumi.Sprintf("Allow group %s to inspect compartments in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage clusters in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage virtual-network-family in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage instance-family in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage volume-family in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage load-balancers in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to use vnics in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to use network-security-groups in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to use private-ips in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage public-ips in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage object-family in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage tag-namespaces in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to manage tag-defaults in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to use tag-namespaces in compartment %s", b.BootstrapGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow group %s to use subnets in compartment %s", b.BootstrapGroupName, b.CompartmentName),
		},
	}, pulumi.DeleteBeforeReplace(true), pulumi.DependsOn([]pulumi.Resource{compartment}))
	if err != nil {
		return fmt.Errorf("failed to create bootstrap policy: %v", err)
	}

	// Create operational policy in root compartment
	operationalPolicy, err := pulumiIdentity.NewPolicy(ctx, "threeport-operational-policy", &pulumiIdentity.PolicyArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-operational-policy-%s", b.InstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport operational policy for %s", b.InstanceName)),
		Statements: pulumi.StringArray{
			pulumi.Sprintf("Allow group %s to inspect compartments in compartment %s", b.OperationalGroupName, b.CompartmentName),
		},
	}, pulumi.DeleteBeforeReplace(true), pulumi.DependsOn([]pulumi.Resource{compartment}))
	if err != nil {
		return fmt.Errorf("failed to create operational policy: %v", err)
	}

	// Create dynamic group policy in root compartment
	dynamicGroupPolicy, err := pulumiIdentity.NewPolicy(ctx, "threeport-dynamic-group-policy", &pulumiIdentity.PolicyArgs{
		CompartmentId: pulumi.String(b.TenancyOCID),
		Name:          pulumi.String(fmt.Sprintf("threeport-dynamic-group-policy-%s", b.InstanceName)),
		Description:   pulumi.String(fmt.Sprintf("Threeport dynamic group policy for %s", b.InstanceName)),
		Statements: pulumi.StringArray{
			pulumi.Sprintf("Allow dynamic-group %s to manage cluster-family in compartment %s", b.DynamicGroupName, b.CompartmentName),
			pulumi.Sprintf("Allow dynamic-group %s to manage instance-family in compartment %s", b.DynamicGroupName, b.CompartmentName),
		},
	}, pulumi.DeleteBeforeReplace(true), pulumi.DependsOn([]pulumi.Resource{compartment}))
	if err != nil {
		return fmt.Errorf("failed to create dynamic group policy: %v", err)
	}

	// Export all outputs for Stage 2
	ctx.Export("compartmentOCID", compartment.ID())
	ctx.Export("serviceUserOCID", serviceUser.ID())
	ctx.Export("serviceUserAPIKey", apiKey.KeyValue)
	ctx.Export("serviceUserPrivateKey", pulumi.String(keyPair.PrivateKeyPEM))
	ctx.Export("serviceUserFingerprint", pulumi.String(keyPair.Fingerprint))
	ctx.Export("bootstrapGroupOCID", bootstrapGroup.ID())
	ctx.Export("operationalGroupOCID", operationalGroup.ID())
	ctx.Export("dynamicGroupOCID", dynamicGroup.ID())
	ctx.Export("bootstrapPolicyOCID", bootstrapPolicy.ID())
	ctx.Export("operationalPolicyOCID", operationalPolicy.ID())
	ctx.Export("dynamicGroupPolicyOCID", dynamicGroupPolicy.ID())

	return nil
}

// RunStage1Bootstrap executes the Stage 1 Pulumi bootstrap
func (b *OCIBootstrapPulumi) RunStage1Bootstrap() (*BootstrapOutputs, error) {
	ctx := context.Background()

	fmt.Printf("Using home region '%s' for bootstrap operations (global resources)\n", b.HomeRegion)
	fmt.Printf("Target region '%s' will be used for infrastructure deployment\n", b.TargetRegion)

	// Set up Pulumi workspace using shared utility
	stack, err := SetupPulumiWorkspace(&PulumiWorkspaceConfig{
		ProjectName:   "bootstrap",
		InstanceName:  fmt.Sprintf("bootstrap-%s", b.InstanceName),
		Program:       b.bootstrapPulumiProgram,
		Region:        b.HomeRegion, // Use home region for bootstrap
		ConfigProfile: "DEFAULT",    // Use DEFAULT profile for bootstrap
		TenancyOCID:   b.TenancyOCID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to setup Pulumi workspace: %v", err)
	}

	// Run pulumi up
	fmt.Printf("Running Stage 1 bootstrap in home region: %s\n", b.HomeRegion)
	upRes, err := stack.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		// Don't trigger cleanup for Pulumi errors - they're handled by Pulumi's own state management
		return nil, fmt.Errorf("failed to run pulumi up (resources may be partially created): %v", err)
	}

	// Extract outputs
	outputs := &BootstrapOutputs{
		CompartmentOCID:        upRes.Outputs["compartmentOCID"].Value.(string),
		ServiceUserOCID:        upRes.Outputs["serviceUserOCID"].Value.(string),
		ServiceUserAPIKey:      upRes.Outputs["serviceUserAPIKey"].Value.(string),
		ServiceUserPrivateKey:  upRes.Outputs["serviceUserPrivateKey"].Value.(string),
		ServiceUserFingerprint: upRes.Outputs["serviceUserFingerprint"].Value.(string),
		BootstrapGroupOCID:     upRes.Outputs["bootstrapGroupOCID"].Value.(string),
		OperationalGroupOCID:   upRes.Outputs["operationalGroupOCID"].Value.(string),
		DynamicGroupOCID:       upRes.Outputs["dynamicGroupOCID"].Value.(string),
		BootstrapPolicyOCID:    upRes.Outputs["bootstrapPolicyOCID"].Value.(string),
		OperationalPolicyOCID:  upRes.Outputs["operationalPolicyOCID"].Value.(string),
		DynamicGroupPolicyOCID: upRes.Outputs["dynamicGroupPolicyOCID"].Value.(string),
	}

	// Update OCI configuration with service user profile
	err = b.updateOCIConfiguration(outputs)
	if err != nil {
		return nil, fmt.Errorf("failed to update OCI configuration: %v", err)
	}

	// Wait for API key to be active
	fmt.Println("Waiting for API key to be active...")
	err = b.waitForAPIKeyActive(outputs)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for API key to be active: %v", err)
	}
	fmt.Println("✅ API key is active")

	return outputs, nil
}

// updateOCIConfiguration creates the service user profile in ~/.oci/config
func (b *OCIBootstrapPulumi) updateOCIConfiguration(outputs *BootstrapOutputs) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	configPath := filepath.Join(homeDir, ".oci", "config")

	// Create config content for our service profile
	configContent := fmt.Sprintf(`[THREEPORT_SERVICE]
user=%s
fingerprint=%s
tenancy=%s
region=%s
key_file=%s
`,
		outputs.ServiceUserOCID,
		outputs.ServiceUserFingerprint,
		b.TenancyOCID,
		b.TargetRegion, // Use target region for infrastructure
		filepath.Join(homeDir, ".oci", fmt.Sprintf("threeport-service-%s.pem", b.InstanceName)),
	)

	// If file doesn't exist, create it with default permissions
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			return fmt.Errorf("failed to create OCI config: %w", err)
		}
	} else {
		// File exists, append our section
		existingContent, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read existing OCI config: %w", err)
		}

		// Remove any existing THREEPORT_SERVICE section
		lines := strings.Split(string(existingContent), "\n")
		var newLines []string
		skipSection := false
		for _, line := range lines {
			if strings.TrimSpace(line) == "[THREEPORT_SERVICE]" {
				skipSection = true
				continue
			}
			if skipSection && strings.HasPrefix(strings.TrimSpace(line), "[") {
				skipSection = false
			}
			if !skipSection {
				newLines = append(newLines, line)
			}
		}

		// Append our section
		newContent := strings.Join(newLines, "\n") + "\n" + configContent

		// Write back to file
		if err := os.WriteFile(configPath, []byte(newContent), 0600); err != nil {
			return fmt.Errorf("failed to update OCI config: %w", err)
		}
	}

	// Save private key to file only if it doesn't already exist
	privateKeyPath := filepath.Join(homeDir, ".oci", fmt.Sprintf("threeport-service-%s.pem", b.InstanceName))

	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		// File doesn't exist, create it
		err = os.WriteFile(privateKeyPath, []byte(outputs.ServiceUserPrivateKey), 0600)
		if err != nil {
			return fmt.Errorf("failed to write private key file: %v", err)
		}
		fmt.Printf("✅ Created private key: %s\n", privateKeyPath)
	} else {
		// File exists, don't overwrite
		fmt.Printf("ℹ️  Private key already exists, not overwriting: %s\n", privateKeyPath)
	}
	fmt.Printf("✅ Updated OCI config: %s\n", configPath)

	return nil
}

// generateOCIAPIKeyPair generates a real RSA key pair for OCI API authentication using existing utilities
func generateOCIAPIKeyPair() (*APIKeyPair, error) {
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
	fingerprint := generateOCIFingerprint(publicKeyDER)

	return &APIKeyPair{
		PublicKeyPEM:  publicKeyPEM,
		PrivateKeyPEM: privateKeyPEM,
		Fingerprint:   fingerprint,
	}, nil
}

// generateOCIFingerprint creates an MD5 fingerprint for OCI API keys
func generateOCIFingerprint(publicKeyDER []byte) string {
	// OCI uses MD5 hash of the DER-encoded public key for fingerprint
	hash := md5.Sum(publicKeyDER)

	// Format as colon-separated hex string
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x:%02x",
		hash[0], hash[1], hash[2], hash[3], hash[4], hash[5], hash[6], hash[7],
		hash[8], hash[9], hash[10], hash[11], hash[12], hash[13], hash[14], hash[15])
}

// GetServiceUserProfileName returns the OCI config profile name for the service user
func (b *OCIBootstrapPulumi) GetServiceUserProfileName() string {
	return "THREEPORT_SERVICE"
}

// waitForAPIKeyActive waits for the API key to be active and usable
func (b *OCIBootstrapPulumi) waitForAPIKeyActive(outputs *BootstrapOutputs) error {
	// Create identity client with service user credentials
	configProvider := common.NewRawConfigurationProvider(
		b.TenancyOCID,
		outputs.ServiceUserOCID,
		b.HomeRegion,
		outputs.ServiceUserFingerprint,
		outputs.ServiceUserPrivateKey,
		nil,
	)

	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return fmt.Errorf("failed to create identity client: %w", err)
	}

	// Wait for API key to be active (max 5 minutes)
	maxAttempts := 60 // 60 * 5 seconds = 5 minutes
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Try to list API keys
		request := identity.ListApiKeysRequest{
			UserId: &outputs.ServiceUserOCID,
		}
		response, err := identityClient.ListApiKeys(context.Background(), request)
		if err == nil && len(response.Items) > 0 {
			// Found API keys, check if our key is active
			for _, key := range response.Items {
				if *key.Fingerprint == outputs.ServiceUserFingerprint {
					return nil // Key is active
				}
			}
		}

		if attempt < maxAttempts {
			time.Sleep(5 * time.Second)
		}
	}

	return fmt.Errorf("timed out waiting for API key to become active")
}

// DeleteOCIBootstrapResources deletes the bootstrap resources for a given instance
func DeleteOCIBootstrapResources(instanceName string) error {
	// Create a new Pulumi workspace with the bootstrap stack
	bootstrap, err := NewOCIBootstrapPulumi(instanceName, "")
	if err != nil {
		return fmt.Errorf("failed to create bootstrap instance: %w", err)
	}

	// Set up Pulumi workspace and get stack
	stack, err := SetupPulumiWorkspace(&PulumiWorkspaceConfig{
		ProjectName:   "bootstrap",
		InstanceName:  fmt.Sprintf("bootstrap-%s", instanceName),
		Program:       bootstrap.bootstrapPulumiProgram,
		Region:        bootstrap.HomeRegion,
		ConfigProfile: "DEFAULT",
		TenancyOCID:   bootstrap.TenancyOCID,
	})
	if err != nil {
		return fmt.Errorf("failed to setup Pulumi workspace: %w", err)
	}

	// Destroy the stack
	ctx := context.Background()
	_, err = stack.Destroy(ctx)
	if err != nil {
		return fmt.Errorf("failed to destroy bootstrap stack: %w", err)
	}

	return nil
}

// CleanupOCILocalFiles removes local OCI files created during bootstrap
func CleanupOCILocalFiles() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	ociDir := filepath.Join(homeDir, ".oci")
	files, err := os.ReadDir(ociDir)
	if err != nil {
		return fmt.Errorf("failed to read .oci directory: %w", err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), "threeport-service-") {
			filePath := filepath.Join(ociDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("failed to remove file %s: %w", filePath, err)
			}
		}
	}

	return nil
}

// getAPIKeyPairFromPrivateKey generates the public key and fingerprint from an existing private key
func getAPIKeyPairFromPrivateKey(privateKeyPEM string) (*APIKeyPair, error) {
	// Parse the private key
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Generate public key from private key
	publicKey := &privateKey.PublicKey

	// Convert public key to PEM format
	publicKeyDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %v", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	// Calculate fingerprint (MD5 hash of the public key DER)
	hash := md5.Sum(publicKeyDER)
	fingerprint := strings.ToLower(hex.EncodeToString(hash[:]))

	// Format fingerprint with colons
	var formattedFingerprint strings.Builder
	for i, char := range fingerprint {
		if i > 0 && i%2 == 0 {
			formattedFingerprint.WriteString(":")
		}
		formattedFingerprint.WriteRune(char)
	}

	return &APIKeyPair{
		PrivateKeyPEM: privateKeyPEM,
		PublicKeyPEM:  string(publicKeyPEM),
		Fingerprint:   formattedFingerprint.String(),
	}, nil
}
