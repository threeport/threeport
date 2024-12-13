package v0

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BuildBinary builds the go binary for a threeport control plane component.
func BuildBinary(
	threeportPath string,
	arch string,
	binName string,
	mainPath string,
	noCache bool,
) error {
	// construct build arguments
	buildArgs := []string{"build"}

	// append build flags
	buildArgs = append(buildArgs, "-gcflags=\\\"all=-N -l\\\"") // escape quotes and escape char for shell

	// append no cache flag if specified
	if noCache {
		buildArgs = append(buildArgs, "-a")
	}

	// append output flag
	buildArgs = append(buildArgs, "-o")

	// append binary name
	buildArgs = append(buildArgs, "bin/"+binName)

	// append main.go filepath
	buildArgs = append(buildArgs, mainPath)

	fmt.Printf("go %s \n", strings.Join(buildArgs, " "))

	// construct build command
	cmd := exec.Command("go", buildArgs...)
	cmd.Env = os.Environ()
	goEnv := []string{
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH=" + arch,
	}
	cmd.Env = append(cmd.Env, goEnv...)
	cmd.Dir = threeportPath

	// start build command
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build %s with output '%s': %w", binName, string(output), err)
	}

	return nil
}

// BuildImage builds a container image for the linux platform and optionally
// pushes to a registry and/or loads the image to a kind cluster
func BuildImage(
	threeportPath string,
	dockerfilePath string,
	arch string,
	imageRepo string,
	imageName string,
	imageTag string,
	pushImage bool,
	loadImage bool,
	loadClusterName string,
) error {
	image := fmt.Sprintf("%s/%s:%s", imageRepo, imageName, imageTag)

	dockerBuildCmd := exec.Command(
		"docker",
		"buildx",
		"build",
		"--load",
		fmt.Sprintf("--platform=linux/%s", arch),
		"-t",
		image,
		"-f",
		dockerfilePath,
		threeportPath,
	)

	output, err := dockerBuildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("image build failed for %s with output '%s': %w", image, string(output), err)
	}

	fmt.Printf("%s image built\n", image)

	// push image if pushImage=true
	if pushImage {
		dockerPushCmd := exec.Command("docker", "push", image)

		output, err = dockerPushCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("image push failed for %s with output '%s': %w", image, string(output), err)
		}

		fmt.Printf("%s image pushed\n", image)
	}

	// load image if loadImage=true
	if loadImage {
		kindLoadCmd := exec.Command(
			"kind",
			"load",
			"docker-image",
			image,
			"--name",
			loadClusterName,
		)

		output, err = kindLoadCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf(
				"failed to load image %s to kind cluster with output '%s': %w",
				image,
				string(output),
				err,
			)
		}

		fmt.Printf("%s image loaded to kind cluster\n", image)
	}

	return nil
}
