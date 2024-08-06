//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// BuildAgent builds the binary for the workload-controller.
func BuildAgent() error {
	buildCmd := exec.Command(
		"go",
		"build",
		"-o",
		"bin/agent",
		"cmd/agent/main.go",
	)

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for agent with output '%s': %w", output, err)
	}

	fmt.Println("agent binary built and available at bin/agent")

	return nil
}

func BuildImage(
	component string,
	imageRepo string,
	imageTag string,
	pushImage bool,
	loadImage bool,
) error {
	dockerBuildCmd := exec.Command(
		"docker",
		"buildx",
		"build",
		"--build-arg",
		fmt.Sprintf("BINARY=%s", component),
		"--target",
		"dev",
		"--load",
		//"--platform=linux/amd64",
		"--platform=darwin/arm64",
		"-t",
		fmt.Sprintf("%s/threeport-%s:%s", imageRepo, component, imageTag),
		"-f",
		"cmd/tptdev/image/Dockerfile",
		"/Users/lander2k2/Projects/src/github.com/threeport/threeport",
	)

	output, err := dockerBuildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("image build failed for %s with output '%s': %w", component, output, err)
	}

	fmt.Printf("%s/threeport-%s:%s image built \n", imageRepo, component, imageTag)

	if pushImage {
		dockerPushCmd := exec.Command(
			"docker",
			"push",
			fmt.Sprintf("%s/threeport-%s:%s", imageRepo, component, imageTag),
		)

		output, err := dockerPushCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("image push for %s failed with output '%s': %w", component, output, err)
		}

		fmt.Printf("%s image pushed\n", component)
	}

	return nil
}

// BuildSDK builds SDK binary and installs in GOPATH
func BuildSDK() error {
	goPath := os.Getenv("GOPATH")
	outputPath := filepath.Join(goPath, "bin", "threeport-sdk")

	sdkCmd := exec.Command(
		"go",
		"build",
		"-o",
		outputPath,
		"cmd/sdk/main.go",
	)

	output, err := sdkCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for sdk binary with output: '%s': %w", output, err)
	}

	fmt.Println("sdk binary built and available at bin/threeport-sdk")

	return nil
}

// BuildDbMigrator builds database migrator
func BuildDbMigrator() error {

	buildDbCmd := exec.Command(
		"go",
		"build",
		"-o",
		"bin/database-migrator",
		"cmd/database-migrator/main.go",
	)

	output, err := buildDbCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for database migrator with output: '%s': %w", output, err)
	}

	fmt.Println("database migrator built and available at cmd/database-migrator")

	return nil
}

// BuildTptdev builds tptdev binary
func BuildTptdev() error {

	buildTptdevCmd := exec.Command(
		"go",
		"build",
		"-o",
		"bin/tptdev",
		"cmd/tptdev/main.go",
	)
	output, err := buildTptdevCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for tptdev with output: '%s': %w", output, err)
	}

	fmt.Println("tptdev built and available at cmd/tptdev")

	return nil
}

// InstallTptdev installs tptdev binary
func InstallTptdev() error {

	installTptdevCmd := exec.Command(
		"sudo",
		"cp",
		"./bin/tptdev",
		"/usr/local/bin/tptdev",
	)
	output, err := installTptdevCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed for tptdev with output: '%s': %w", output, err)
	}

	fmt.Println("tptdev installed and available at /usr/local/bin/tptdev")

	return nil
}

// BuildTptctl builds tptctl binary
func BuildTptctl() error {

	buildTptctlCmd := exec.Command(
		"go",
		"build",
		"-o",
		"bin/tptctl",
		"cmd/tptctl/main.go",
	)
	output, err := buildTptctlCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for tptctl binary with output: '%s': %w", output, err)
	}

	fmt.Println("tptctl binary built and available at cmd/tptctl")

	return nil
}

// InstallTptctl installs tptctl binary
func InstallTptctl() error {

	installTptctlCmd := exec.Command(
		"sudo",
		"cp",
		"./bin/tptctl",
		"/usr/local/bin/tptctl",
	)
	output, err := installTptctlCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed for tptctl binary with output: '%s': %w", output, err)
	}

	fmt.Println("tptctl binary installed and available at/usr/local/bin/tptctl")

	return nil
}
