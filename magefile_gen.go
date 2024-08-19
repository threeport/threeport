// generated by 'threeport-sdk gen' - do not edit

//go:build mage
// +build mage

package main

import (
	"fmt"
	util "github.com/threeport/threeport/pkg/util/v0"
	"os"
	"os/exec"
	"runtime"
)

// BuildApi builds the REST API binary.
func BuildApi() error {
	workingDir, arch, err := getBuildVals()
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

// BuildApiImage builds and pushes a development REST API image.
func BuildApiImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/rest-api/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-rest-api",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push rest-api image: %w", err)
	}

	return nil
}

// BuildSecretController builds the binary for the secret-controller.
func BuildSecretController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildSecretControllerImage builds and pushes the container image for the secret-controller.
func BuildSecretControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/secret-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-secret-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push secret-controller image: %w", err)
	}

	return nil
}

// BuildAwsController builds the binary for the aws-controller.
func BuildAwsController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildAwsControllerImage builds and pushes the container image for the aws-controller.
func BuildAwsControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/aws-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-aws-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push aws-controller image: %w", err)
	}

	return nil
}

// BuildControlPlaneController builds the binary for the control-plane-controller.
func BuildControlPlaneController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildControlPlaneControllerImage builds and pushes the container image for the control-plane-controller.
func BuildControlPlaneControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/control-plane-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-control-plane-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push control-plane-controller image: %w", err)
	}

	return nil
}

// BuildGatewayController builds the binary for the gateway-controller.
func BuildGatewayController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildGatewayControllerImage builds and pushes the container image for the gateway-controller.
func BuildGatewayControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/gateway-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-gateway-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push gateway-controller image: %w", err)
	}

	return nil
}

// BuildHelmWorkloadController builds the binary for the helm-workload-controller.
func BuildHelmWorkloadController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildHelmWorkloadControllerImage builds and pushes the container image for the helm-workload-controller.
func BuildHelmWorkloadControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/helm-workload-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-helm-workload-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push helm-workload-controller image: %w", err)
	}

	return nil
}

// BuildKubernetesRuntimeController builds the binary for the kubernetes-runtime-controller.
func BuildKubernetesRuntimeController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildKubernetesRuntimeControllerImage builds and pushes the container image for the kubernetes-runtime-controller.
func BuildKubernetesRuntimeControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/kubernetes-runtime-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-kubernetes-runtime-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push kubernetes-runtime-controller image: %w", err)
	}

	return nil
}

// BuildObservabilityController builds the binary for the observability-controller.
func BuildObservabilityController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildObservabilityControllerImage builds and pushes the container image for the observability-controller.
func BuildObservabilityControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/observability-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-observability-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push observability-controller image: %w", err)
	}

	return nil
}

// BuildTerraformController builds the binary for the terraform-controller.
func BuildTerraformController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildTerraformControllerImage builds and pushes the container image for the terraform-controller.
func BuildTerraformControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/terraform-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-terraform-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push terraform-controller image: %w", err)
	}

	return nil
}

// BuildWorkloadController builds the binary for the workload-controller.
func BuildWorkloadController() error {
	workingDir, arch, err := getBuildVals()
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

// BuildWorkloadControllerImage builds and pushes the container image for the workload-controller.
func BuildWorkloadControllerImage() error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		"cmd/workload-controller/image/Dockerfile-alpine",
		arch,
		"localhost:5001",
		"threeport-workload-controller",
		"dev",
		true,
		false,
		"",
	); err != nil {
		return fmt.Errorf("failed to build and push workload-controller image: %w", err)
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

// LoadImage builds and loads an image to the provided kind cluster.
func LoadImage(kindClusterName string, component string) error {
	workingDir, arch, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get build values: %w", err)
	}

	if err := util.BuildImage(
		workingDir,
		fmt.Sprintf("cmd/%s/image/Dockerfile-alpine", component),
		arch,
		"localhost:5001",
		fmt.Sprintf("threeport-%s", component),
		"dev",
		false,
		true,
		kindClusterName,
	); err != nil {
		return fmt.Errorf("failed to build and load image: %w", err)
	}

	return nil
}

// Docs generates the API server documentation that is served by the API
func Docs() error {
	docsDestination := "pkg/api-server/v0/docs"
	swagCmd := exec.Command(
		"swag",
		"init",
		"--dir",
		"cmd/rest-api,pkg/api,pkg/api-server/v0",
		"--parseDependency",
		"--generalInfo",
		"main_gen.go",
		"--output",
		docsDestination,
	)

	output, err := swagCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("API docs generation failed with output '%s': %w", output, err)
	}

	fmt.Printf("API docs generated in %s\n", docsDestination)

	return nil
}

// getBuildVals returns the working directory and arch for builds.
func getBuildVals() (string, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get working directory: %w", err)
	}

	arch := runtime.GOARCH

	return workingDir, arch, nil
}
