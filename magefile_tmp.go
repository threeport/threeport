// temporary magefile to use pending work being merged in

//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// BuildApi builds the REST API binary.
func BuildApi() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"rest-api",
		"cmd/rest-api/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build rest-api binary: %w", err)
	}

	fmt.Println("binary built and available at bin/rest-api")

	return nil
}

// BuildSecretController builds the binary for the secret-controller.
func BuildSecretController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"secret-controller",
		"cmd/secret-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build secret-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/secret-controller")

	return nil
}

// BuildAwsController builds the binary for the aws-controller.
func BuildAwsController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"aws-controller",
		"cmd/aws-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build aws-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/aws-controller")

	return nil
}

// BuildControlPlaneController builds the binary for the control-plane-controller.
func BuildControlPlaneController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"control-plane-controller",
		"cmd/control-plane-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build control-plane-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/control-plane-controller")

	return nil
}

// BuildGatewayController builds the binary for the gateway-controller.
func BuildGatewayController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"gateway-controller",
		"cmd/gateway-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build gateway-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/gateway-controller")

	return nil
}

// BuildHelmWorkloadController builds the binary for the helm-workload-controller.
func BuildHelmWorkloadController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"helm-workload-controller",
		"cmd/helm-workload-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build helm-workload-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/helm-workload-controller")

	return nil
}

// BuildKubernetesRuntimeController builds the binary for the kubernetes-runtime-controller.
func BuildKubernetesRuntimeController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"kubernetes-runtime-controller",
		"cmd/kubernetes-runtime-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build kubernetes-runtime-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/kubernetes-runtime-controller")

	return nil
}

// BuildObservabilityController builds the binary for the observability-controller.
func BuildObservabilityController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"observability-controller",
		"cmd/observability-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build observability-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/observability-controller")

	return nil
}

// BuildTerraformController builds the binary for the terraform-controller.
func BuildTerraformController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"terraform-controller",
		"cmd/terraform-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build terraform-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/terraform-controller")

	return nil
}

// BuildWorkloadController builds the binary for the workload-controller.
func BuildWorkloadController() error {
	workingDir, arch, err := GetBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildBinary(
		workingDir,
		arch,
		"workload-controller",
		"cmd/workload-controller/main_gen.go",
		false,
	); err != nil {
		return fmt.Errorf("failed to build workload-controller binary: %w", err)
	}

	fmt.Println("binary built and available at bin/workload-controller")

	return nil
}

// BuildApiImage builds and pushes the REST API image.
func BuildApiImage() error {
	if err := DevImage(
		"rest-api",
		"localhost:5001",
		"threeport-rest-api",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push rest-api image: %w", err)
	}

	return nil
}

// BuildSecretControllerImage builds and pushes the container image for the secret-controller.
func BuildSecretControllerImage() error {
	if err := DevImage(
		"secret-controller",
		"localhost:5001",
		"threeport-secret-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "secret-controller", err)
	}

	return nil
}

// BuildAwsControllerImage builds and pushes the container image for the aws-controller.
func BuildAwsControllerImage() error {
	if err := DevImage(
		"aws-controller",
		"localhost:5001",
		"threeport-aws-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "aws-controller", err)
	}

	return nil
}

// BuildControlPlaneControllerImage builds and pushes the container image for the control-plane-controller.
func BuildControlPlaneControllerImage() error {
	if err := DevImage(
		"control-plane-controller",
		"localhost:5001",
		"threeport-control-plane-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "control-plane-controller", err)
	}

	return nil
}

// BuildGatewayControllerImage builds and pushes the container image for the gateway-controller.
func BuildGatewayControllerImage() error {
	if err := DevImage(
		"gateway-controller",
		"localhost:5001",
		"threeport-gateway-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "gateway-controller", err)
	}

	return nil
}

// BuildHelmWorkloadControllerImage builds and pushes the container image for the helm-workload-controller.
func BuildHelmWorkloadControllerImage() error {
	if err := DevImage(
		"helm-workload-controller",
		"localhost:5001",
		"threeport-helm-workload-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "helm-workload-controller", err)
	}

	return nil
}

// BuildKubernetesRuntimeControllerImage builds and pushes the container image for the kubernetes-runtime-controller.
func BuildKubernetesRuntimeControllerImage() error {
	if err := DevImage(
		"kubernetes-runtime-controller",
		"localhost:5001",
		"threeport-kubernetes-runtime-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "kubernetes-runtime-controller", err)
	}

	return nil
}

// BuildObservabilityControllerImage builds and pushes the container image for the observability-controller.
func BuildObservabilityControllerImage() error {
	if err := DevImage(
		"observability-controller",
		"localhost:5001",
		"threeport-observability-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "observability-controller", err)
	}

	return nil
}

// BuildTerraformControllerImage builds and pushes the container image for the terraform-controller.
func BuildTerraformControllerImage() error {
	if err := DevImage(
		"terraform-controller",
		"localhost:5001",
		"threeport-terraform-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "terraform-controller", err)
	}

	return nil
}

// BuildWorkloadControllerImage builds and pushes the container image for the workload-controller.
func BuildWorkloadControllerImage() error {
	if err := DevImage(
		"workload-controller",
		"localhost:5001",
		"threeport-workload-controller",
		"dev",
		true,
		false,
	); err != nil {
		return fmt.Errorf("failed to build and push %s image: %w", "workload-controller", err)
	}

	return nil
}

// BuildAll builds the binaries for all components.
func BuildAll() error {
	if err := BuildApi(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildSecretController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildAwsController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildControlPlaneController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildGatewayController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildHelmWorkloadController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildKubernetesRuntimeController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildObservabilityController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildTerraformController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	if err := BuildWorkloadController(); err != nil {
		return fmt.Errorf("failed to build binary: %w", err)
	}

	return nil
}

// BuildAllImages builds and pushes images for all components.
func BuildAllImages() error {
	if err := BuildApiImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildSecretControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildAwsControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildControlPlaneControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildGatewayControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildHelmWorkloadControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildKubernetesRuntimeControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildObservabilityControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildTerraformControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	if err := BuildWorkloadControllerImage(); err != nil {
		return fmt.Errorf("failed to build and push image: %w", err)
	}

	return nil
}

// DevImage builds and pushes a container image using the alpine
// Dockerfile.
func DevImage(
	component string,
	imageRepo string,
	imageName string,
	imageTag string,
	pushImage bool,
	loadImage bool,
) error {
	rootDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory for image build: %w", err)
	}

	image := fmt.Sprintf("%s/%s:%s", imageRepo, imageName, imageTag)

	dockerBuildCmd := exec.Command(
		"docker",
		"buildx",
		"build",
		"--load",
		fmt.Sprintf("--platform=linux/%s", runtime.GOARCH),
		"-t",
		image,
		"-f",
		fmt.Sprintf("cmd/%s/image/Dockerfile-alpine", component),
		rootDir,
	)

	output, err := dockerBuildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("image build failed for %s with output '%s': %w", component, output, err)
	}

	fmt.Printf("%s image built\n", image)

	if pushImage {
		dockerPushCmd := exec.Command("docker", "push", image)

		output, err = dockerPushCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("image push for %s failed with output '%s': %w", component, output, err)
		}

		fmt.Printf("%s image pushed\n", image)
	}

	// TODO: load image if loadImage=true

	return nil
}

// GetBuildVals returns the working directory and arch for bin builds.
func GetBuildVals() (string, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get working directory: %w", err)
	}

	arch := runtime.GOARCH

	return workingDir, arch, nil
}
