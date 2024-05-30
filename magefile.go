//go:build mage
// +build mage

package main

import (
	"fmt"
	"os/exec"
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
