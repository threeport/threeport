//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// BuildAgent builds the binary for the agent.
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
	buildCmd := exec.Command(
		"go",
		"build",
		"-o",
		"bin/database-migrator",
		"cmd/database-migrator/main.go",
	)

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for database-migrator with output '%s': %w", output, err)
	}

	fmt.Println("database-migrator binary built and available at bin/agent")

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
