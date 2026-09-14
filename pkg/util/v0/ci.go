package v0

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// WriteCIEnv prints KEY=value lines for a workflow GITHUB_ENV file. It always
// prints GOFLAGS. When moduleVersion is non-empty it also prints
// MODULE_IMAGE_TAG from ResolveImageTag.
func WriteCIEnv(moduleVersion string) error {
	fmt.Printf("GOFLAGS=-p=%d\n", BuildParallelism())
	if moduleVersion == "" {
		return nil
	}

	tag, err := ResolveImageTag(".", moduleVersion)
	if err != nil {
		return fmt.Errorf("failed to resolve module image tag: %w", err)
	}
	fmt.Printf("MODULE_IMAGE_TAG=%s\n", tag)
	return nil
}

// TeardownCILeftovers removes leftover kind clusters, containers, networks,
// and volumes. registryDown is optional.
func TeardownCILeftovers(tptctlBin, planeName string, registryDown func() error) error {
	teardownStep(tptctlBin, "down", "-n", planeName)
	teardownStep("kind", "delete", "clusters", "--all")
	if home, err := os.UserHomeDir(); err == nil {
		teardownStep("rm", "-f", filepath.Join(home, ".config", "tptctl", "config.yaml"))
	}
	if registryDown != nil {
		if err := registryDown(); err != nil {
			fmt.Fprintf(os.Stderr, "ci:teardown: remove local registry: %v\n", err)
		}
	}
	teardownStep("docker", "container", "prune", "-f", "--filter", "label=io.x-k8s.kind.cluster")
	teardownStep("docker", "network", "prune", "-f")
	teardownStep("docker", "volume", "prune", "-f")
	return nil
}

func teardownStep(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ci:teardown: %s: %v\n", name, err)
	}
}
