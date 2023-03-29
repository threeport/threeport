package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultDevName = "dev-0"
)

// kindClusterName returns a kind cluster name for a threeport control plane
// install.
func kindClusterName(name string) string {
	return fmt.Sprintf("threeport-%s", name)
}

// defaultKubeconfig returns the path to the user's default kubeconfig.
func defaultKubeconfig() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to user's home directory: %w", err)
	}

	return filepath.Join(homeDir, ".kube", "config"), nil
}
