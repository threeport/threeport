package provider

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetPulumiStateDir returns the base directory for all Threeport Pulumi state
// (~/.threeport/pulumi-state). Each runtime instance uses a subdirectory; see
// GetPulumiRuntimeStateDir.
func GetPulumiStateDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".threeport", "pulumi-state"), nil
}

// GetPulumiRuntimeStateDir returns the Pulumi workspace directory for a given
// runtime instance (~/.threeport/pulumi-state/<runtimeInstanceName>). It does
// not create the directory; callers that need it on disk should use os.MkdirAll.
func GetPulumiRuntimeStateDir(runtimeInstanceName string) (string, error) {
	base, err := GetPulumiStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, runtimeInstanceName), nil
}
