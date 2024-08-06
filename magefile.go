//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// BuildAgent builds the binary for the agent.
func BuildAgent() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"agent",
		"cmd/agent/main.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build agent binary: %w", err)
	}

	fmt.Println("binary built and available at bin/agent")

	return nil
}

// BuildAgentImage builds and pushes the agent image.
func BuildAgentImage() error {
	if err := DevImage(
		"agent",
		"localhost:5001",
		"threeport-agent",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push rest-api image: %w", err)
	}

	return nil
}

// BuildDatabaseMigrator builds the binary for the database-migrator.
func BuildDatabaseMigrator() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"database-migrator",
		"cmd/database-migrator/main.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build database-migrator binary: %w", err)
	}

	fmt.Println("binary built and available at bin/database-migrator")

	return nil
}

// BuildDatabaseMigratorImage builds and pushes the database-migrator image.
func BuildDatabaseMigratorImage() error {
	if err := DevImage(
		"database-migrator",
		"localhost:5001",
		"threeport-database-migrator",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push database-migrator image: %w", err)
	}

	return nil
}

// BuildImage builds a container image for a Threeport control plane component
// for the given architecture.
func BuildImage(
	component string,
	imageRepo string,
	imageTag string,
	pushImage bool,
	loadImage bool,
	arch string,
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
		fmt.Sprintf("--platform=d%s", arch),
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


// E2e calls ginkgo to run the e2e tests suite.
func E2e(
	provider string,
	imageRepo string,
	clean bool,
) error {
	// determine path to root of Threeport repo
	threeportPath, err := os.Getwd() // mage must be run from repo root
	if err != nil {
		return fmt.Errorf("failed to get path to Threeport repo: %w", err)
	}

	cmd := "ginkgo"
	args := []string{
		"test/e2e",
		"--",
		"-provider=kind",
		fmt.Sprintf("-image-repo=%s", imageRepo),
		fmt.Sprintf("-threeport-path=%s", threeportPath),
		fmt.Sprintf("-clean=%t", clean),
	}
	if err := util.RunCommandStreamOutput(cmd, args...); err != nil {
		return fmt.Errorf("failed to run e2e tests: %w", err)
	}

return nil
}

// Builds sdk binary and installs in GOPATH

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

// E2eLocal is a wrapper for E2e that uses kind, a local image repo in a docker
// container and cleans up at completion.
func E2eLocal() error {
	return E2e("kind", "local", true)
}

// E2eClean removes the kind cluster and local container registry for e2e
// testing.
func E2eClean() error {
	cmd := "kind"
	args := []string{
		"delete",
		"cluster",
		"-n",
		"threeport-e2e-test",
	}
	if err := util.RunCommandStreamOutput(cmd, args...); err != nil {
		return fmt.Errorf("failed to remove e2e test cluster: %w", err)
	}

	if err := CleanLocalRegistry(); err != nil {
		return err
	}

return nil
}

// Builds database migrator

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


// Integration runs integration tests against an existing Threeport control
// plane.
func Integration() error {
	cmd := "go"
	args := []string{
		"test",
		"-v",
		"./test/integration",
		"-count=1",
	}
	if err := util.RunCommandStreamOutput(cmd, args...); err != nil {
		return fmt.Errorf("failed to run integration tests: %w", err)
	}

	return nil
}

// CreateLocalRegistry starts a docker container to serve as a local container
// registry.
func CreateLocalRegistry() error {
	if err := tptdev.CreateLocalRegistry(); err != nil {
		return fmt.Errorf("failed to create local container registry: %w", err)
	}

	return nil
}

// CleanLocalRegistry stops and removes the local container registry.
func CleanLocalRegistry() error {
	if err := tptdev.DeleteLocalRegistry(); err != nil {
		return fmt.Errorf("failed to remove local container registry: %w", err)
	}

return nil 
}

// Builds tptdev binary

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
