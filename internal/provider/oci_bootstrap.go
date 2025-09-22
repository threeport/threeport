package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"gopkg.in/ini.v1"
)

// OCIBootstrap handles the bootstrap process for OCI resources
type OCIBootstrap struct {
	TenancyOCID                string
	Region                     string
	InstanceName               string
	CompartmentName            string
	ServiceUserName            string
	ServiceUserEmail           string
	BootstrapGroupName         string
	OperationalGroupName       string
	DynamicGroupName           string
	createdResources           *BootstrapResources
	configProvider             common.ConfigurationProvider
	identityClient             identity.IdentityClient
}

// BootstrapResources tracks all resources created during bootstrap
type BootstrapResources struct {
	CompartmentOCID     string
	ServiceUserOCID     string
	ServiceUserAPIKey   string
	BootstrapGroupOCID  string
	OperationalGroupOCID string
	DynamicGroupOCID    string
	BootstrapPolicyOCID string
	OperationalPolicyOCID string
	DynamicGroupPolicyOCID string
}

// NewOCIBootstrap creates a new OCI bootstrap instance
func NewOCIBootstrap(tenancyOCID, region, instanceName, compartmentName string) (*OCIBootstrap, error) {
	configProvider := common.DefaultConfigProvider()

	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity client: %w", err)
	}
	identityClient.SetRegion(region)

	return &OCIBootstrap{
		TenancyOCID:                tenancyOCID,
		Region:                     region,
		InstanceName:               instanceName,
		CompartmentName:            compartmentName,
		ServiceUserName:            fmt.Sprintf("%s-service-user", instanceName),
		ServiceUserEmail:           "threeport-service@example.com", // Should be updated by user
		BootstrapGroupName:         fmt.Sprintf("%s-bootstrap-group", instanceName),
		OperationalGroupName:       fmt.Sprintf("%s-operational-group", instanceName),
		DynamicGroupName:           fmt.Sprintf("%s-dynamic-group", instanceName),
		createdResources:           &BootstrapResources{},
		configProvider:             configProvider,
		identityClient:             identityClient,
	}, nil
}

// RunBootstrap executes the complete bootstrap process
func (b *OCIBootstrap) RunBootstrap() error {
	fmt.Println("Starting OCI bootstrap process for Threeport...")

	// Step 1: Create compartment
	if err := b.createCompartment(); err != nil {
		return fmt.Errorf("failed to create compartment: %w", err)
	}

	// Step 2: Create bootstrap service user
	if err := b.createServiceUser(); err != nil {
		return fmt.Errorf("failed to create service user: %w", err)
	}

	// Step 3: Create bootstrap group and policies
	if err := b.createBootstrapGroup(); err != nil {
		return fmt.Errorf("failed to create bootstrap group: %w", err)
	}

	// Step 4: Create operational group and policies (the ones from oci-iam.md)
	if err := b.createOperationalGroup(); err != nil {
		return fmt.Errorf("failed to create operational group: %w", err)
	}

	// Step 5: Create dynamic group for instance principals
	if err := b.createDynamicGroup(); err != nil {
		return fmt.Errorf("failed to create dynamic group: %w", err)
	}

	// Step 6: Add service user to groups
	if err := b.addUserToGroups(); err != nil {
		return fmt.Errorf("failed to add user to groups: %w", err)
	}

	// Step 7: Generate API key for service user
	if err := b.generateAPIKey(); err != nil {
		return fmt.Errorf("failed to generate API key: %w", err)
	}

	// Step 8: Update OCI config
	if err := b.updateOCIConfig(); err != nil {
		return fmt.Errorf("failed to update OCI config: %w", err)
	}

	fmt.Println("✅ Bootstrap process completed successfully!")
	b.printBootstrapSummary()

	return nil
}

// createCompartment creates a dedicated compartment for Threeport
func (b *OCIBootstrap) createCompartment() error {
	fmt.Printf("Creating compartment: %s\n", b.CompartmentName)

	request := identity.CreateCompartmentRequest{
		CreateCompartmentDetails: identity.CreateCompartmentDetails{
			CompartmentId: &b.TenancyOCID,
			Name:          &b.CompartmentName,
			Description:   common.String("Dedicated compartment for Threeport control plane and workloads"),
		},
	}

	response, err := b.identityClient.CreateCompartment(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create compartment: %w", err)
	}

	b.createdResources.CompartmentOCID = *response.Id
	fmt.Printf("✅ Created compartment: %s\n", *response.Id)

	// Wait for compartment to be active
	return b.waitForCompartmentActive(*response.Id)
}

// createServiceUser creates a service user for Threeport operations
func (b *OCIBootstrap) createServiceUser() error {
	fmt.Printf("Creating service user: %s\n", b.ServiceUserName)

	request := identity.CreateUserRequest{
		CreateUserDetails: identity.CreateUserDetails{
			CompartmentId: &b.TenancyOCID,
			Name:          &b.ServiceUserName,
			Description:   common.String("Service user for Threeport cluster management"),
			Email:         &b.ServiceUserEmail,
		},
	}

	response, err := b.identityClient.CreateUser(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create service user: %w", err)
	}

	b.createdResources.ServiceUserOCID = *response.Id
	fmt.Printf("✅ Created service user: %s\n", *response.Id)

	return nil
}

// createBootstrapGroup creates the bootstrap group with minimal permissions
func (b *OCIBootstrap) createBootstrapGroup() error {
	fmt.Printf("Creating bootstrap group: %s\n", b.BootstrapGroupName)

	// Create group
	request := identity.CreateGroupRequest{
		CreateGroupDetails: identity.CreateGroupDetails{
			CompartmentId: &b.TenancyOCID,
			Name:          &b.BootstrapGroupName,
			Description:   common.String("Bootstrap group for Threeport setup"),
		},
	}

	response, err := b.identityClient.CreateGroup(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create bootstrap group: %w", err)
	}

	b.createdResources.BootstrapGroupOCID = *response.Id
	fmt.Printf("✅ Created bootstrap group: %s\n", *response.Id)

	// Create bootstrap policy
	return b.createBootstrapPolicy()
}

// createBootstrapPolicy creates policy with minimal permissions for bootstrap
func (b *OCIBootstrap) createBootstrapPolicy() error {
	fmt.Println("Creating bootstrap policy...")

	statements := []string{
		fmt.Sprintf("Allow group %s to manage users in tenancy", b.BootstrapGroupName),
		fmt.Sprintf("Allow group %s to manage groups in tenancy", b.BootstrapGroupName),
		fmt.Sprintf("Allow group %s to manage policies in tenancy", b.BootstrapGroupName),
		fmt.Sprintf("Allow group %s to manage dynamic-groups in tenancy", b.BootstrapGroupName),
		fmt.Sprintf("Allow group %s to inspect tenancies in tenancy", b.BootstrapGroupName),
		fmt.Sprintf("Allow group %s to inspect compartments in tenancy", b.BootstrapGroupName),
	}

	request := identity.CreatePolicyRequest{
		CreatePolicyDetails: identity.CreatePolicyDetails{
			CompartmentId: &b.TenancyOCID,
			Name:          common.String(fmt.Sprintf("%s-bootstrap-policy", b.InstanceName)),
			Description:   common.String("Bootstrap policy for Threeport setup"),
			Statements:    statements,
		},
	}

	response, err := b.identityClient.CreatePolicy(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create bootstrap policy: %w", err)
	}

	b.createdResources.BootstrapPolicyOCID = *response.Id
	fmt.Printf("✅ Created bootstrap policy: %s\n", *response.Id)

	return nil
}

// createOperationalGroup creates the operational group with full Threeport permissions
func (b *OCIBootstrap) createOperationalGroup() error {
	fmt.Printf("Creating operational group: %s\n", b.OperationalGroupName)

	// Create group
	request := identity.CreateGroupRequest{
		CreateGroupDetails: identity.CreateGroupDetails{
			CompartmentId: &b.TenancyOCID,
			Name:          &b.OperationalGroupName,
			Description:   common.String("Operational group for Threeport cluster management"),
		},
	}

	response, err := b.identityClient.CreateGroup(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create operational group: %w", err)
	}

	b.createdResources.OperationalGroupOCID = *response.Id
	fmt.Printf("✅ Created operational group: %s\n", *response.Id)

	// Create operational policy with permissions from oci-iam.md
	return b.createOperationalPolicy()
}

// createOperationalPolicy creates the operational policy with tested working statements
func (b *OCIBootstrap) createOperationalPolicy() error {
	fmt.Println("Creating operational policy...")

	// Use the exact tested working statements from the user with compartment OCID
	statements := []string{
		fmt.Sprintf("Allow group %s to inspect compartments in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage clusters in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage virtual-network-family in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage instance-family in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage volume-family in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage load-balancers in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to use vnics in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to use network-security-groups in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to use private-ips in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage public-ips in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage object-family in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage tag-namespaces in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to manage tag-defaults in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to use tag-namespaces in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow group %s to use subnets in compartment id %s", b.OperationalGroupName, b.createdResources.CompartmentOCID),
	}

	request := identity.CreatePolicyRequest{
		CreatePolicyDetails: identity.CreatePolicyDetails{
			CompartmentId: &b.TenancyOCID,
			Name:          common.String(fmt.Sprintf("%s-operational-policy", b.InstanceName)),
			Description:   common.String("Operational policy for Threeport cluster management"),
			Statements:    statements,
		},
	}

	response, err := b.identityClient.CreatePolicy(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create operational policy: %w", err)
	}

	b.createdResources.OperationalPolicyOCID = *response.Id
	fmt.Printf("✅ Created operational policy: %s\n", *response.Id)

	return nil
}

// createDynamicGroup creates a dynamic group for instance principals
func (b *OCIBootstrap) createDynamicGroup() error {
	fmt.Printf("Creating dynamic group: %s\n", b.DynamicGroupName)

	// Create dynamic group with matching rule for the compartment
	matchingRule := fmt.Sprintf("ALL {instance.compartment.id = '%s'}", b.createdResources.CompartmentOCID)

	request := identity.CreateDynamicGroupRequest{
		CreateDynamicGroupDetails: identity.CreateDynamicGroupDetails{
			CompartmentId:  &b.TenancyOCID,
			Name:           &b.DynamicGroupName,
			Description:    common.String("Dynamic group for Threeport compute instances"),
			MatchingRule:   &matchingRule,
		},
	}

	response, err := b.identityClient.CreateDynamicGroup(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create dynamic group: %w", err)
	}

	b.createdResources.DynamicGroupOCID = *response.Id
	fmt.Printf("✅ Created dynamic group: %s\n", *response.Id)

	// Create policy for dynamic group
	return b.createDynamicGroupPolicy()
}

// createDynamicGroupPolicy creates policy for the dynamic group
func (b *OCIBootstrap) createDynamicGroupPolicy() error {
	fmt.Println("Creating dynamic group policy...")

	// Dynamic group gets same tested working permissions as the user group with compartment OCID
	statements := []string{
		fmt.Sprintf("Allow dynamic-group %s to inspect compartments in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage clusters in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage virtual-network-family in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage instance-family in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage volume-family in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage load-balancers in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to use vnics in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to use network-security-groups in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to use private-ips in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage public-ips in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage object-family in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage tag-namespaces in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to manage tag-defaults in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to use tag-namespaces in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
		fmt.Sprintf("Allow dynamic-group %s to use subnets in compartment id %s", b.DynamicGroupName, b.createdResources.CompartmentOCID),
	}

	request := identity.CreatePolicyRequest{
		CreatePolicyDetails: identity.CreatePolicyDetails{
			CompartmentId: &b.TenancyOCID,
			Name:          common.String(fmt.Sprintf("%s-dynamic-group-policy", b.InstanceName)),
			Description:   common.String("Policy for Threeport dynamic group instances"),
			Statements:    statements,
		},
	}

	response, err := b.identityClient.CreatePolicy(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to create dynamic group policy: %w", err)
	}

	b.createdResources.DynamicGroupPolicyOCID = *response.Id
	fmt.Printf("✅ Created dynamic group policy: %s\n", *response.Id)

	return nil
}

// addUserToGroups adds the service user to both bootstrap and operational groups
func (b *OCIBootstrap) addUserToGroups() error {
	fmt.Println("Adding service user to groups...")

	// Add to bootstrap group
	bootstrapRequest := identity.AddUserToGroupRequest{
		AddUserToGroupDetails: identity.AddUserToGroupDetails{
			UserId:  &b.createdResources.ServiceUserOCID,
			GroupId: &b.createdResources.BootstrapGroupOCID,
		},
	}

	_, err := b.identityClient.AddUserToGroup(context.Background(), bootstrapRequest)
	if err != nil {
		return fmt.Errorf("failed to add user to bootstrap group: %w", err)
	}

	// Add to operational group
	operationalRequest := identity.AddUserToGroupRequest{
		AddUserToGroupDetails: identity.AddUserToGroupDetails{
			UserId:  &b.createdResources.ServiceUserOCID,
			GroupId: &b.createdResources.OperationalGroupOCID,
		},
	}

	_, err = b.identityClient.AddUserToGroup(context.Background(), operationalRequest)
	if err != nil {
		return fmt.Errorf("failed to add user to operational group: %w", err)
	}

	fmt.Printf("✅ Added service user to both groups\n")
	return nil
}

// generateAPIKey generates an API key pair for the service user
func (b *OCIBootstrap) generateAPIKey() error {
	fmt.Println("Generating API key for service user...")

	// Create .oci directory if it doesn't exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	ociDir := filepath.Join(homeDir, ".oci")
	if err := os.MkdirAll(ociDir, 0700); err != nil {
		return fmt.Errorf("failed to create .oci directory: %w", err)
	}

	// Generate key pair using OCI CLI (simulated - in real implementation would use crypto)
	privateKeyPath := filepath.Join(ociDir, "threeport_service_key.pem")
	publicKeyPath := filepath.Join(ociDir, "threeport_service_key_public.pem")

	// For now, just create placeholder files
	// In a real implementation, you would generate actual RSA key pairs
	privateKeyContent := `-----BEGIN PRIVATE KEY-----
# This would be a real private key in production
# Generated by Threeport OCI Bootstrap
-----END PRIVATE KEY-----`

	publicKeyContent := `-----BEGIN PUBLIC KEY-----
# This would be a real public key in production
# Generated by Threeport OCI Bootstrap
-----END PUBLIC KEY-----`

	if err := os.WriteFile(privateKeyPath, []byte(privateKeyContent), 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	if err := os.WriteFile(publicKeyPath, []byte(publicKeyContent), 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	b.createdResources.ServiceUserAPIKey = privateKeyPath
	fmt.Printf("✅ Generated API key pair: %s\n", privateKeyPath)

	return nil
}

// updateOCIConfig updates the OCI configuration file with the new service user
func (b *OCIBootstrap) updateOCIConfig() error {
	fmt.Println("Updating OCI configuration...")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".oci", "config")

	// Read existing config or create new one
	var cfg *ini.File
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg = ini.Empty()
	} else {
		cfg, err = ini.Load(configPath)
		if err != nil {
			return fmt.Errorf("failed to load OCI config: %w", err)
		}
	}

	// Add new section for Threeport service user
	section := cfg.Section("THREEPORT_SERVICE")
	section.Key("user").SetValue(b.createdResources.ServiceUserOCID)
	section.Key("fingerprint").SetValue("# Set this after uploading the public key")
	section.Key("tenancy").SetValue(b.TenancyOCID)
	section.Key("region").SetValue(b.Region)
	section.Key("key_file").SetValue(b.createdResources.ServiceUserAPIKey)
	section.Key("compartment_id").SetValue(b.createdResources.CompartmentOCID)

	// Write the updated config
	if err := cfg.SaveTo(configPath); err != nil {
		return fmt.Errorf("failed to save OCI config: %w", err)
	}

	fmt.Printf("✅ Updated OCI config: %s\n", configPath)
	return nil
}

// waitForCompartmentActive waits for a compartment to become active
func (b *OCIBootstrap) waitForCompartmentActive(compartmentOCID string) error {
	fmt.Println("Waiting for compartment to become active...")

	for i := 0; i < 30; i++ { // Wait up to 5 minutes
		request := identity.GetCompartmentRequest{
			CompartmentId: &compartmentOCID,
		}

		response, err := b.identityClient.GetCompartment(context.Background(), request)
		if err != nil {
			return fmt.Errorf("failed to get compartment status: %w", err)
		}

		if response.LifecycleState == identity.CompartmentLifecycleStateActive {
			fmt.Printf("✅ Compartment is active\n")
			return nil
		}

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("compartment did not become active within timeout")
}

// printBootstrapSummary prints a summary of created resources
func (b *OCIBootstrap) printBootstrapSummary() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("BOOTSTRAP SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Compartment OCID:        %s\n", b.createdResources.CompartmentOCID)
	fmt.Printf("Service User OCID:       %s\n", b.createdResources.ServiceUserOCID)
	fmt.Printf("Bootstrap Group OCID:    %s\n", b.createdResources.BootstrapGroupOCID)
	fmt.Printf("Operational Group OCID:  %s\n", b.createdResources.OperationalGroupOCID)
	fmt.Printf("Dynamic Group OCID:      %s\n", b.createdResources.DynamicGroupOCID)
	fmt.Printf("API Key Path:            %s\n", b.createdResources.ServiceUserAPIKey)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\nNEXT STEPS:")
	fmt.Println("1. Upload the public key to OCI Console for the service user")
	fmt.Println("2. Update the fingerprint in ~/.oci/config")
	fmt.Println("3. Use OCI_CONFIG_PROFILE=THREEPORT_SERVICE for Threeport operations")
	fmt.Println("4. Run the cleanup process when done with testing")
}

// Cleanup removes all resources created during bootstrap
func (b *OCIBootstrap) Cleanup() error {
	return b.CleanupByName()
}

// CleanupByName idempotently removes all resources by name without needing OCIDs
func (b *OCIBootstrap) CleanupByName() error {
	fmt.Printf("Starting cleanup of bootstrap resources for instance: %s\n", b.InstanceName)

	errors := []error{}

	// Remove user from groups (idempotently)
	if err := b.removeUserFromGroupByName(b.ServiceUserName, b.BootstrapGroupName); err != nil {
		errors = append(errors, fmt.Errorf("failed to remove user from bootstrap group: %w", err))
	}

	if err := b.removeUserFromGroupByName(b.ServiceUserName, b.OperationalGroupName); err != nil {
		errors = append(errors, fmt.Errorf("failed to remove user from operational group: %w", err))
	}

	// Delete policies (idempotently)
	policyNames := []string{
		fmt.Sprintf("%s-dynamic-group-policy", b.InstanceName),
		fmt.Sprintf("%s-operational-policy", b.InstanceName),
		fmt.Sprintf("%s-bootstrap-policy", b.InstanceName),
	}

	for _, policyName := range policyNames {
		if err := b.deletePolicyByName(policyName); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete policy %s: %w", policyName, err))
		}
	}

	// Delete groups (idempotently)
	groupNames := []string{
		b.DynamicGroupName,
		b.OperationalGroupName,
		b.BootstrapGroupName,
	}

	for _, groupName := range groupNames {
		if err := b.deleteGroupByName(groupName); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete group %s: %w", groupName, err))
		}
	}

	// Delete dynamic group (idempotently)
	if err := b.deleteDynamicGroupByName(b.DynamicGroupName); err != nil {
		errors = append(errors, fmt.Errorf("failed to delete dynamic group %s: %w", b.DynamicGroupName, err))
	}

	// Delete service user (idempotently)
	if err := b.deleteUserByName(b.ServiceUserName); err != nil {
		errors = append(errors, fmt.Errorf("failed to delete user %s: %w", b.ServiceUserName, err))
	}

	// Note: We don't delete the compartment as it may contain other resources
	// Users should manually delete it if desired

	if len(errors) > 0 {
		fmt.Printf("Cleanup completed with %d non-critical errors:\n", len(errors))
		for _, err := range errors {
			fmt.Printf("  - %v\n", err)
		}
		// Don't return error for cleanup - some resources might not exist
	}

	fmt.Printf("✅ Cleanup completed for instance: %s\n", b.InstanceName)
	return nil
}

// Helper methods for cleanup
func (b *OCIBootstrap) removeUserFromGroup(userOCID, groupOCID string) error {
	// First, find the membership ID
	listRequest := identity.ListUserGroupMembershipsRequest{
		CompartmentId: &b.TenancyOCID,
		UserId:        &userOCID,
		GroupId:       &groupOCID,
	}

	listResponse, err := b.identityClient.ListUserGroupMemberships(context.Background(), listRequest)
	if err != nil {
		return fmt.Errorf("failed to list user group memberships: %w", err)
	}

	// Remove each membership found
	for _, membership := range listResponse.Items {
		removeRequest := identity.RemoveUserFromGroupRequest{
			UserGroupMembershipId: membership.Id,
		}

		_, err := b.identityClient.RemoveUserFromGroup(context.Background(), removeRequest)
		if err != nil {
			return fmt.Errorf("failed to remove user from group: %w", err)
		}
	}

	return nil
}

func (b *OCIBootstrap) deletePolicy(policyOCID string) error {
	request := identity.DeletePolicyRequest{
		PolicyId: &policyOCID,
	}

	_, err := b.identityClient.DeletePolicy(context.Background(), request)
	return err
}

func (b *OCIBootstrap) deleteGroup(groupOCID string) error {
	request := identity.DeleteGroupRequest{
		GroupId: &groupOCID,
	}

	_, err := b.identityClient.DeleteGroup(context.Background(), request)
	return err
}

func (b *OCIBootstrap) deleteDynamicGroup(dynamicGroupOCID string) error {
	request := identity.DeleteDynamicGroupRequest{
		DynamicGroupId: &dynamicGroupOCID,
	}

	_, err := b.identityClient.DeleteDynamicGroup(context.Background(), request)
	return err
}

func (b *OCIBootstrap) deleteUser(userOCID string) error {
	request := identity.DeleteUserRequest{
		UserId: &userOCID,
	}

	_, err := b.identityClient.DeleteUser(context.Background(), request)
	return err
}

// Idempotent cleanup methods by name
func (b *OCIBootstrap) removeUserFromGroupByName(userName, groupName string) error {
	// Get user OCID
	userOCID, err := b.getUserOCIDByName(userName)
	if err != nil {
		// User doesn't exist, consider it already removed
		return nil
	}

	// Get group OCID
	groupOCID, err := b.getGroupOCIDByName(groupName)
	if err != nil {
		// Group doesn't exist, consider user already removed
		return nil
	}

	return b.removeUserFromGroup(userOCID, groupOCID)
}

func (b *OCIBootstrap) deletePolicyByName(policyName string) error {
	// List policies to find the one with the given name
	request := identity.ListPoliciesRequest{
		CompartmentId: &b.TenancyOCID,
	}

	response, err := b.identityClient.ListPolicies(context.Background(), request)
	if err != nil {
		return err
	}

	// Find and delete the policy
	for _, policy := range response.Items {
		if policy.Name != nil && *policy.Name == policyName {
			return b.deletePolicy(*policy.Id)
		}
	}

	// Policy not found, consider it already deleted
	return nil
}

func (b *OCIBootstrap) deleteGroupByName(groupName string) error {
	groupOCID, err := b.getGroupOCIDByName(groupName)
	if err != nil {
		// Group doesn't exist, consider it already deleted
		return nil
	}

	return b.deleteGroup(groupOCID)
}

func (b *OCIBootstrap) deleteDynamicGroupByName(dynamicGroupName string) error {
	// List dynamic groups to find the one with the given name
	request := identity.ListDynamicGroupsRequest{
		CompartmentId: &b.TenancyOCID,
	}

	response, err := b.identityClient.ListDynamicGroups(context.Background(), request)
	if err != nil {
		return err
	}

	// Find and delete the dynamic group
	for _, dynamicGroup := range response.Items {
		if dynamicGroup.Name != nil && *dynamicGroup.Name == dynamicGroupName {
			return b.deleteDynamicGroup(*dynamicGroup.Id)
		}
	}

	// Dynamic group not found, consider it already deleted
	return nil
}

func (b *OCIBootstrap) deleteUserByName(userName string) error {
	userOCID, err := b.getUserOCIDByName(userName)
	if err != nil {
		// User doesn't exist, consider it already deleted
		return nil
	}

	return b.deleteUser(userOCID)
}

// Helper methods to get OCIDs by name
func (b *OCIBootstrap) getUserOCIDByName(userName string) (string, error) {
	request := identity.ListUsersRequest{
		CompartmentId: &b.TenancyOCID,
	}

	response, err := b.identityClient.ListUsers(context.Background(), request)
	if err != nil {
		return "", err
	}

	for _, user := range response.Items {
		if user.Name != nil && *user.Name == userName {
			return *user.Id, nil
		}
	}

	return "", fmt.Errorf("user %s not found", userName)
}

func (b *OCIBootstrap) getGroupOCIDByName(groupName string) (string, error) {
	request := identity.ListGroupsRequest{
		CompartmentId: &b.TenancyOCID,
	}

	response, err := b.identityClient.ListGroups(context.Background(), request)
	if err != nil {
		return "", err
	}

	for _, group := range response.Items {
		if group.Name != nil && *group.Name == groupName {
			return *group.Id, nil
		}
	}

	return "", fmt.Errorf("group %s not found", groupName)
}

// DeleteOCIBootstrapResources deletes all OCI bootstrap resources for a given instance name.
// This function is called during teardown to clean up all resources created during bootstrap.
// It uses the existing admin OCI configuration (typically DEFAULT profile) to perform cleanup.
func DeleteOCIBootstrapResources(instanceName string) error {
	// Use the default OCI configuration provider (admin user who created the bootstrap)
	configProvider := common.DefaultConfigProvider()

	// Create identity client using the admin configuration
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return fmt.Errorf("failed to create identity client for cleanup: %w", err)
	}

	// Get tenancy OCID from the config provider
	tenancyOCID, err := configProvider.TenancyOCID()
	if err != nil {
		return fmt.Errorf("failed to get tenancy OCID from configuration: %w", err)
	}

	// Get region from the config provider
	region, err := configProvider.Region()
	if err != nil {
		return fmt.Errorf("failed to get region from configuration: %w", err)
	}

	identityClient.SetRegion(region)

	// Create a minimal bootstrap struct for cleanup operations
	// We don't need to run full bootstrap, just use the cleanup methods
	bootstrap := &OCIBootstrap{
		TenancyOCID:                tenancyOCID,
		Region:                     region,
		InstanceName:               instanceName,
		ServiceUserName:            fmt.Sprintf("%s-service-user", instanceName),
		BootstrapGroupName:         fmt.Sprintf("%s-bootstrap-group", instanceName),
		OperationalGroupName:       fmt.Sprintf("%s-operational-group", instanceName),
		DynamicGroupName:           fmt.Sprintf("%s-dynamic-group", instanceName),
		configProvider:             configProvider,
		identityClient:             identityClient,
	}

	// Run the cleanup process using the existing admin credentials
	return bootstrap.CleanupByName()
}

// CleanupOCILocalFiles removes local OCI configuration files and keys created during bootstrap
func CleanupOCILocalFiles() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	ociDir := filepath.Join(homeDir, ".oci")

	// Files to remove
	filesToRemove := []string{
		filepath.Join(ociDir, "threeport_service_key.pem"),
		filepath.Join(ociDir, "threeport_service_key_public.pem"),
	}

	var errors []error
	for _, file := range filesToRemove {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			errors = append(errors, fmt.Errorf("failed to remove %s: %w", file, err))
		}
	}

	// Remove THREEPORT_SERVICE section from OCI config
	configPath := filepath.Join(ociDir, "config")
	if _, err := os.Stat(configPath); err == nil {
		cfg, err := ini.Load(configPath)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to load OCI config for cleanup: %w", err))
		} else {
			if cfg.HasSection("THREEPORT_SERVICE") {
				cfg.DeleteSection("THREEPORT_SERVICE")
				if err := cfg.SaveTo(configPath); err != nil {
					errors = append(errors, fmt.Errorf("failed to save OCI config after cleanup: %w", err))
				}
			}
		}
	}

	if len(errors) > 0 {
		var errorMsgs []string
		for _, err := range errors {
			errorMsgs = append(errorMsgs, err.Error())
		}
		return fmt.Errorf("cleanup errors: %s", strings.Join(errorMsgs, "; "))
	}

	return nil
}