package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// PulumiWorkspaceConfig contains configuration for setting up a Pulumi workspace
type PulumiWorkspaceConfig struct {
	ProjectName    string
	InstanceName   string
	Program        pulumi.RunFunc
	Region         string
	ConfigProfile  string
	TenancyOCID    string
	UserOCID       string
	Fingerprint    string
	PrivateKeyPath string
}

// SetupPulumiWorkspace creates a Pulumi workspace with local state backend
func SetupPulumiWorkspace(config *PulumiWorkspaceConfig) (auto.Stack, error) {
	// Set up state directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, ".threeport", "pulumi-state", config.InstanceName)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create state directory: %w", err)
	}

	// Set Pulumi environment variables
	if err := setPulumiEnvVars(stateDir, config.ProjectName); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	// Create Pulumi.yaml project file
	pulumiYaml := fmt.Sprintf(`name: %s
runtime: go
description: %s project for Threeport
`, config.ProjectName, config.ProjectName)
	pulumiYamlPath := filepath.Join(stateDir, "Pulumi.yaml")
	if err := os.WriteFile(pulumiYamlPath, []byte(pulumiYaml), 0644); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create Pulumi.yaml: %w", err)
	}

	ctx := context.Background()

	// Create workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.Program(config.Program),
		auto.WorkDir(stateDir),
	)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Create or select stack
	stackName := fmt.Sprintf("threeport-%s-%s", config.ProjectName, config.InstanceName)
	stack, err := auto.UpsertStack(ctx, stackName, workspace)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create/select stack: %w", err)
	}

	// Set up stack configuration
	err = stack.SetConfig(ctx, "oci:region", auto.ConfigValue{Value: config.Region})
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set region config: %w", err)
	}

	// Set config profile if provided
	if config.ConfigProfile != "" {
		err = stack.SetConfig(ctx, "oci:configFileProfile", auto.ConfigValue{Value: config.ConfigProfile})
		if err != nil {
			return auto.Stack{}, fmt.Errorf("failed to set config profile: %w", err)
		}
	}

	// Set explicit OCI credentials if provided (for service user authentication)
	if config.TenancyOCID != "" {
		err = stack.SetConfig(ctx, "oci:tenancyOcid", auto.ConfigValue{Value: config.TenancyOCID})
		if err != nil {
			return auto.Stack{}, fmt.Errorf("failed to set tenancy OCID config: %w", err)
		}
	}

	if config.UserOCID != "" {
		err = stack.SetConfig(ctx, "oci:userOcid", auto.ConfigValue{Value: config.UserOCID})
		if err != nil {
			return auto.Stack{}, fmt.Errorf("failed to set user OCID config: %w", err)
		}
	}

	if config.Fingerprint != "" {
		err = stack.SetConfig(ctx, "oci:fingerprint", auto.ConfigValue{Value: config.Fingerprint})
		if err != nil {
			return auto.Stack{}, fmt.Errorf("failed to set fingerprint config: %w", err)
		}
	}

	if config.PrivateKeyPath != "" {
		err = stack.SetConfig(ctx, "oci:privateKeyPath", auto.ConfigValue{Value: config.PrivateKeyPath})
		if err != nil {
			return auto.Stack{}, fmt.Errorf("failed to set private key path config: %w", err)
		}
	}

	// Validate that the correct OCI profile is being used
	if config.ConfigProfile != "" {
		err = validateOCIProfile(ctx, stack, config.ConfigProfile)
		if err != nil {
			return auto.Stack{}, fmt.Errorf("OCI profile validation failed: %w", err)
		}
	}

	return stack, nil
}

// setPulumiEnvVars sets up the required Pulumi environment variables
func setPulumiEnvVars(stateDir, projectName string) error {
	os.Setenv("PULUMI_BACKEND_URL", "file://"+stateDir)
	os.Setenv("PULUMI_HOME", stateDir)
	os.Setenv("PULUMI_ORGANIZATION", "organization")
	os.Setenv("PULUMI_PROJECT", projectName)
	os.Setenv("PULUMI_CONFIG_PASSPHRASE", "threeport")

	// Set plugin path to the default location
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	defaultPluginPath := filepath.Join(userHomeDir, ".pulumi", "plugins")
	os.Setenv("PULUMI_PLUGIN_PATH", defaultPluginPath)

	return nil
}

// validateOCIProfile validates that Pulumi is configured to use the expected OCI profile
func validateOCIProfile(ctx context.Context, stack auto.Stack, expectedProfile string) error {
	// Get the configured profile from Pulumi stack
	configuredProfile, err := stack.GetConfig(ctx, "oci:configFileProfile")
	if err != nil {
		return fmt.Errorf("failed to get oci:configFileProfile from stack: %w", err)
	}

	if configuredProfile.Value != expectedProfile {
		return fmt.Errorf("expected OCI profile '%s' but found '%s' in Pulumi stack configuration", expectedProfile, configuredProfile.Value)
	}

	// Validate that the profile exists in the OCI config file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".oci", "config")
	if err := validateOCIProfileExists(configPath, expectedProfile); err != nil {
		return fmt.Errorf("OCI profile validation failed: %w", err)
	}

	// Try to create a config provider with the expected profile to ensure it works
	if err := validateOCIProfileCredentials(configPath, expectedProfile); err != nil {
		return fmt.Errorf("OCI profile credentials validation failed: %w", err)
	}

	fmt.Printf("✅ Validated that Pulumi is using OCI profile: %s\n", expectedProfile)
	return nil
}

// validateOCIProfileExists checks if the specified profile exists in the OCI config file
func validateOCIProfileExists(configPath, profileName string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read OCI config file %s: %w", configPath, err)
	}

	profileSection := fmt.Sprintf("[%s]", profileName)
	if !strings.Contains(string(content), profileSection) {
		return fmt.Errorf("OCI profile '%s' not found in config file %s", profileName, configPath)
	}

	return nil
}

// validateOCIProfileCredentials validates that the OCI profile credentials work
func validateOCIProfileCredentials(configPath, profileName string) error {
	fmt.Printf("  → Validating profile '%s' from config file: %s\n", profileName, configPath)
	
	// Force re-read the config file from disk to avoid caching issues
	// Create a fresh config provider that reads from disk each time
	configProvider := common.CustomProfileConfigProvider(configPath, profileName)
	
	// Try to get basic info to validate credentials work
	_, err := configProvider.TenancyOCID()
	if err != nil {
		return fmt.Errorf("failed to get tenancy OCID from profile '%s': %w", profileName, err)
	}

	userOCID, err := configProvider.UserOCID()
	if err != nil {
		return fmt.Errorf("failed to get user OCID from profile '%s': %w", profileName, err)
	}
	
	_, err = configProvider.Region()
	if err != nil {
		return fmt.Errorf("failed to get region from profile '%s': %w", profileName, err)
	}

	fingerprint, err := configProvider.KeyFingerprint()
	if err != nil {
		return fmt.Errorf("failed to get key fingerprint from profile '%s': %w", profileName, err)
	}

	fmt.Printf("  → Profile '%s' config: user=%s, fingerprint=%s\n", 
		profileName, userOCID, fingerprint)

	// For service user profiles, use retry mechanism for API key propagation
	if strings.Contains(profileName, "SERVICE") {
		fmt.Printf("  → Service user profile detected, testing API key with retry mechanism...\n")
		return validateServiceUserWithRetry(configProvider, profileName, userOCID)
	}

	// For regular profiles, do a single validation attempt
	return validateOCICredentialsOnce(configProvider, profileName, userOCID)
}

// validateServiceUserWithRetry validates service user credentials with retry for API key propagation
func validateServiceUserWithRetry(configProvider common.ConfigurationProvider, profileName, userOCID string) error {
	maxAttempts := 24 // 24 attempts * 5 seconds = 2 minutes max (same as existing logic)
	waitSeconds := 5
	
	fmt.Printf("  → Attempting validation with retry (max %d attempts, %ds intervals)...\n", maxAttempts, waitSeconds)
	
	err := util.Retry(maxAttempts, waitSeconds, func() error {
		return validateOCICredentialsOnce(configProvider, profileName, userOCID)
	})
	
	if err != nil {
		return fmt.Errorf("failed to validate service user credentials after %d attempts over %d minutes: %w", 
			maxAttempts, (maxAttempts*waitSeconds)/60, err)
	}
	
	return nil
}

// validateOCICredentialsOnce performs a single validation attempt
func validateOCICredentialsOnce(configProvider common.ConfigurationProvider, profileName, userOCID string) error {
	// Create an identity client and make a simple API call
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return fmt.Errorf("failed to create identity client with profile '%s': %w", profileName, err)
	}

	// Test the credentials by getting user information
	getUserRequest := identity.GetUserRequest{UserId: &userOCID}
	getUserResponse, err := identityClient.GetUser(context.Background(), getUserRequest)
	if err != nil {
		// For retry attempts, we want to see brief progress indicators
		if strings.Contains(profileName, "SERVICE") {
			fmt.Printf("    ⏳ API key not ready yet, retrying...\n")
		}
		return fmt.Errorf("failed to validate credentials for profile '%s' by calling GetUser API: %w", profileName, err)
	}

	fmt.Printf("✅ Validated OCI profile '%s' credentials for user: %s\n", profileName, *getUserResponse.User.Name)
	return nil
}
