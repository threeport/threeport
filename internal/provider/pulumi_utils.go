package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// PulumiWorkspaceConfig contains configuration for setting up a Pulumi workspace
type PulumiWorkspaceConfig struct {
	ProjectName     string
	InstanceName    string
	Program         pulumi.RunFunc
	Region          string
	ConfigProfile   string
	TenancyOCID     string
	UserOCID        string
	Fingerprint     string
	PrivateKeyPath  string
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