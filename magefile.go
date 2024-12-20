//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	version "github.com/threeport/threeport/internal/version"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// E2e calls ginkgo to run the e2e tests suite.  Takes 2 args: 1. imageRepo -
// either 'local' or the URL for an external image repo.  2. clean - if true
// will remove the control plane and infra after completion.
func (Test) E2e(
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

// E2eLocal is a wrapper for e2e that uses kind, a local image repo in a docker
// container and cleans up at completion.
func (Test) E2eLocal() error {
	test := Test{}
	return test.E2e("local", true)
}

// E2eClean removes the kind cluster and local container registry for e2e
// testing.
func (Test) E2eClean() error {
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

	dev := Dev{}
	if err := dev.LocalRegistryDown(); err != nil {
		return err
	}

	return nil
}

// Sdk builds the SDK binary and installs in GOPATH.
func (Install) Sdk() error {
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

	fmt.Println("sdk binary built and available at $GOPATH/bin/threeport-sdk")

	return nil
}

// Integration runs integration tests against an existing Threeport control
// plane.
func (Test) Integration() error {
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

// Tptdev builds tptdev binary.
func (Build) Tptdev() error {
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

	fmt.Println("tptdev built and available at bin/tptdev")

	return nil
}

// Tptdev installs the tptdev binary at /usr/local/bin/.
func (Install) Tptdev() error {
	build := Build{}
	if err := build.Tptdev(); err != nil {
		return fmt.Errorf("failed to build tptdev: %w", err)
	}

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

// Tptctl builds tptctl binary.
func (Build) Tptctl(goos, goarch string) error {
	buildTptctlCmd := exec.Command(
		"go",
		"build",
		"-o",
		"bin/tptctl",
		"cmd/tptctl/main.go",
	)

	buildTptctlCmd.Env = append(
		os.Environ(),
		fmt.Sprintf("GOOS=%s", goos),
		fmt.Sprintf("GOARCH=%s", goarch),
	)
	output, err := buildTptctlCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for tptctl binary with output: '%s': %w", output, err)
	}

	fmt.Println("tptctl binary built and available at bin/tptctl")

	return nil
}

//// Tptctl builds tptctl binary.
//func (Build) TptctlDev() error {
//	buildTptctlCmd := exec.Command(
//		"go",
//		"build",
//		"-o",
//		"bin/tptctl",
//		"cmd/tptctl/main.go",
//	)
//	output, err := buildTptctlCmd.CombinedOutput()
//	if err != nil {
//		return fmt.Errorf("build failed for tptctl binary with output: '%s': %w", output, err)
//	}
//
//	fmt.Println("tptctl binary built and available at bin/tptctl")
//
//	return nil
//}

// Tptctl installs the tptctl binary at the provided path.
func (Install) Tptctl(path string) error {
	build := Build{}
	if err := build.Tptctl("darwin", "arm64"); err != nil {
		return fmt.Errorf("failed to build tptctl: %w", err)
	}

	installTptctlCmd := exec.Command(
		"cp",
		"./bin/tptctl",
		path,
	)
	output, err := installTptctlCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed for tptctl binary with output: '%s': %w", output, err)
	}

	fmt.Printf("tptctl binary installed and available at %s\n", path)

	return nil
}

// Generate runs runs threeport-sdk code generation and generates API
// swagger docs.
func (Dev) Generate() error {
	dev := Dev{}
	err := dev.GenerateCode()
	if err != nil {
		return fmt.Errorf("code generation failed: %w", err)
	}

	err = dev.GenerateDocs()
	if err != nil {
		return fmt.Errorf("docs generation failed: %w", err)
	}

	fmt.Println("code generated successfully")

	return nil
}

// GenerateCode generates code with threeport-sdk.
func (Dev) GenerateCode() error {
	generateCode := exec.Command(
		"threeport-sdk",
		"gen",
		"-c",
		"sdk-config.yaml",
	)
	output, err := generateCode.CombinedOutput()
	if err != nil {
		return fmt.Errorf("code generation failed with output: '%s': %w", output, err)
	}

	fmt.Println("code generated successfully")

	return nil
}

// GenerateDocs generates the swagger docs served by the API.
func (Dev) GenerateDocs() error {
	docsDestination := "pkg/api-server/v0/docs"
	generateSwaggerDocs := exec.Command(
		"swag",
		"init",
		"--dir",
		"cmd/rest-api,pkg/api-server/v0,pkg/api-server/v0",
		"--parseDependency",
		"--generalInfo",
		"main_gen.go",
		"--output",
		docsDestination,
	)

	output, err := generateSwaggerDocs.CombinedOutput()
	if err != nil {
		return fmt.Errorf("swagger docs generation failed with output: '%s': %w", output, err)
	}

	fmt.Printf("API swagger docs generated successfully in %s\n", docsDestination)

	return nil
}

// Commits checks to make sure commit messages follow conventional commits
// format.
func (Test) Commits() error {
	testCommits := exec.Command(
		"test/scripts/commit-check-latest.sh",
	)

	output, err := testCommits.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run commit check: '%s': %w", output, err)
	}

	fmt.Println("commit check ran successfully")

	return nil
}

// Up spins up a control plane using tptctl and a local registry for testing.
func (Test) Up() error {
	testUp := exec.Command(
		"./bin/tptctl",
		"up",
		"-r",
		installer.DevImageRepo,
		"-t",
		version.GetVersion(),
		"-n",
		"dev-0",
		"--local-registry",
	)

	output, err := testUp.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create test control plane: '%s': %w", output, err)
	}

	fmt.Println("local test control plane created")

	return nil
}

// Up spins up a local development environment.
func (Dev) Up() error {
	devUp := exec.Command(
		"./bin/tptdev",
		"up",
		"--auth-enabled=false",
	)

	output, err := devUp.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create local dev environment: '%s': %w", output, err)
	}

	fmt.Println("local dev environment ran successfully")

	return nil
}

// Down removes the local development environment.
func (Dev) Down() error {
	devDown := exec.Command(
		"./bin/tptdev",
		"down",
	)
	output, err := devDown.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete local dev environment: '%s': %w", output, err)
	}

	fmt.Println("local dev environment successfully deleted")

	return nil
}

// ForwardApi forwards local port 1323 to the local dev API.
func (Dev) ForwardApi() error {
	devforwardAPI := exec.Command(
		"kubectl",
		"port-forward",
		"-n",
		"threeport-control-plane",
		"service/threeport-api-server",
		"1323:80",
	)
	output, err := devforwardAPI.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to forward local port 1323 to local dev API: '%s': %w", output, err)
	}

	fmt.Println("local port 1323 forwarded to local dev API successfully")

	return nil
}

// ForwardCrdb forwards local port 26257 to local dev cockroach database.
func (Dev) ForwardCrdb() error {
	devforwardCrdb := exec.Command(
		"kubectl",
		"port-forward",
		"-n",
		"threeport-control-plane",
		"service/crdb",
		"26257",
	)
	output, err := devforwardCrdb.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to forward local port 26257 to local dev cockroach database: '%s': %w", output, err)
	}

	fmt.Println("local port 26257 forwarded to local dev API successfully")

	return nil
}

// ForwardNats forwards local port 33993 to the local dev API nats server.
func (Dev) ForwardNats() error {
	devforwardNats := exec.Command(
		"kubectl",
		"port-forward",
		"-n",
		"threeport-control-plane",
		"service/nats-js",
		"4222:4222",
	)
	output, err := devforwardNats.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to forward local port 33993 to local dev API nats server: '%s': %w", output, err)
	}

	fmt.Println("local port 33993 forwarded to local dev API nats server successfully")

	return nil
}

// ServeDocs serves the Threeport documentation locally.
func (Dev) ServeDocs() error {
	workingDir, _, err := getBuildVals()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	cmd := "mkdocs"
	args := []string{
		"serve",
		"--config-file",
		fmt.Sprintf("%s/docs/mkdocs.yml", workingDir),
	}
	if err := util.RunCommandStreamOutput(cmd, args...); err != nil {
		return fmt.Errorf("failed to serve docs locally: %w", err)
	}

	return nil
}
