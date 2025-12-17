package provider

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetPulumiStateDir returns the directory where Pulumi state is stored for Threeport.
func GetPulumiStateDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".threeport", "pulumi-state"), nil
}
