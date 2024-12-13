package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	threeportName   = "e2e-test"
	imageTag        = "test"
	threeportConfig = "/tmp/e2e-threeport-config.yaml"
)

// setup operations
var _ = BeforeSuite(func() {
	GinkgoWriter.Write([]byte("beginning setup for test suite"))

	By("building tptctl and tptdev")
	err := buildCli()
	Expect(err).NotTo(HaveOccurred(), "failed to build tptctl and tptdev")

	By("creating the local container registry")
	err = createLocalRegistry()
	Expect(err).NotTo(HaveOccurred(), "failed to create local container registry")

	By("building the container images")
	err = buildContainerImages()
	Expect(err).NotTo(HaveOccurred(), "failed to build and push container images")

	By("provisioning genesis control plane")
	GinkgoWriter.Write([]byte("provisioning control plane..."))
	err = provisionControlPlane()
	Expect(err).NotTo(HaveOccurred(), "failed to provision genesis control plane")
})

// cleanup operations
var _ = AfterSuite(func() {
	if clean {
		GinkgoWriter.Write([]byte("beginning cleanup for test suite"))

		By("remove genesis control plane")
		err := removeControlPlane()
		Expect(err).NotTo(HaveOccurred(), "failed to remove control plane")

		By("remove the local container registry")
		err = removeLocalRegistry()
		Expect(err).NotTo(HaveOccurred(), "failed to create local container registry")
	}
})

// test suite
var _ = Describe("GenesisControlPlane", func() {

	GinkgoWriter.Println("running workload tests...")
	Context("testing workloads", func() {
		It("should manage workloads correctly", func() {

			testCases := &workloadTestCases

			GinkgoWriter.Println("creating test workloads...")
			for _, testCase := range *testCases {
				err := testCase.Create(threeportPath)
				Expect(
					testCase.Worked(err)).To(Equal(true),
					fmt.Sprintf(
						"\nTest case name: %s\nTest case object: %s\nTest case config file path: %s\nTest case deployment name: %s\nTest case expected to work: %t\n",
						testCase.Name,
						testCase.Object,
						testCase.ConfigPath,
						testCase.DeploymentName,
						testCase.ShouldWork,
					),
				)
			}

			GinkgoWriter.Println("describing test workloads...")
			for _, testCase := range *testCases {
				if testCase.ShouldWork {
					err := testCase.Describe(threeportPath, testCases)
					Expect(
						testCase.Worked(err)).To(Equal(true),
						fmt.Sprintf(
							"\nTest case name: %s\nTest case object: %s\nTest case config file path: %s\nTest case deployment name: %s\nTest case expected to work: %t\nError: %v",
							testCase.Name,
							testCase.Object,
							testCase.ConfigPath,
							testCase.DeploymentName,
							testCase.ShouldWork,
							err,
						),
					)
				}
			}

			GinkgoWriter.Println("validating test workloads...")
			for _, testCase := range *testCases {
				if testCase.ShouldWork {
					GinkgoWriter.Printf("validating test case %s...\n", testCase.Name)
					err := testCase.Validate()
					Expect(
						testCase.Worked(err)).To(Equal(true),
						fmt.Sprintf(
							"\nTest case name: %s\nTest case object: %s\nTest case config file path: %s\nTest case deployment name: %s\nTest case expected to work: %t\nError: %v",
							testCase.Name,
							testCase.Object,
							testCase.ConfigPath,
							testCase.DeploymentName,
							testCase.ShouldWork,
							err,
						),
					)
				}
			}

			GinkgoWriter.Println("ensure definitions cannot be deleted with derived instances...")
			for _, testCase := range *testCases {
				if testCase.ShouldWork && testCase.Object == "workload-definition" {
					err := testCase.DeleteDefinitions()
					Expect(
						testCase.Worked(err)).To(Equal(false),
						fmt.Sprintf(
							"\nTest case name: %s\nTest case object: %s\nTest case config file path: %s\nTest case deployment name: %s\nTest case expected to work: %t\nError: %v",
							testCase.Name,
							testCase.Object,
							testCase.ConfigPath,
							testCase.DeploymentName,
							testCase.ShouldWork,
							err,
						),
					)
				}
			}

			GinkgoWriter.Println("deleting test workloads...")
			for _, testCase := range *testCases {
				if testCase.ShouldWork && testCase.Object != "workload-definition" {
					err := testCase.DeleteInstances()
					Expect(
						testCase.Worked(err)).To(Equal(true),
						fmt.Sprintf(
							"\nTest case name: %s\nTest case object: %s\nTest case config file path: %s\nTest case deployment name: %s\nTest case expected to work: %t\nError: %v",
							testCase.Name,
							testCase.Object,
							testCase.ConfigPath,
							testCase.DeploymentName,
							testCase.ShouldWork,
							err,
						),
					)
				}
			}

			GinkgoWriter.Println("ensure definitions can now be deleted with derived instances removed...")
			for _, testCase := range *testCases {
				if testCase.ShouldWork && testCase.Object == "workload-definition" {
					err := testCase.DeleteDefinitions()
					Expect(
						testCase.Worked(err)).To(Equal(true),
						fmt.Sprintf(
							"\nTest case name: %s\nTest case object: %s\nTest case config file path: %s\nTest case deployment name: %s\nTest case expected to work: %t\nError: %v",
							testCase.Name,
							testCase.Object,
							testCase.ConfigPath,
							testCase.DeploymentName,
							testCase.ShouldWork,
							err,
						),
					)
				}
			}
		})
	})
})

// buildCli builds the tptctl and tptdev CLIs.
func buildCli() error {
	tptctlBuildArgs := []string{
		"build",
		"-o",
		filepath.Join(threeportPath, "bin", "tptctl"),
		filepath.Join(threeportPath, "cmd", "tptctl", "main.go"),
	}

	tptctlBuildCmd := exec.Command("go", tptctlBuildArgs...)
	tptctlBuildOutput, err := tptctlBuildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build tptctl with output %s: %w", tptctlBuildOutput, err)
	}

	tptdevBuildArgs := []string{
		"build",
		"-o",
		filepath.Join(threeportPath, "bin", "tptdev"),
		filepath.Join(threeportPath, "cmd", "tptdev", "main.go"),
	}

	tptdevBuildCmd := exec.Command("go", tptdevBuildArgs...)
	tptdevBuildOutput, err := tptdevBuildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build tptdev with output %s: %w", tptdevBuildOutput, err)
	}

	return nil
}

// createLocalRegistry creates a local container registry for testing.
func createLocalRegistry() error {
	if err := tptdev.CreateLocalRegistry(); err != nil {
		return fmt.Errorf("failed to create local container registry: %w", err)
	}

	return nil
}

// buildContainerImages builds all the images for the Threeport control plane.
func buildContainerImages() error {
	buildCmdArgs := []string{
		"build",
		"-r",
		getImageRepo(imageRepo),
		"-t",
		imageTag,
		"--push",
	}
	if err := runCommandStreamOutput(
		threeportPath,
		filepath.Join(threeportPath, "bin", "tptdev"),
		buildCmdArgs...,
	); err != nil {
		return fmt.Errorf("failed to build and push container images: %w", err)
	}

	return nil
}

// provisionControlPlane runs tptctl up and connects the local registry to the
// control plane cluster when it is created if running locally.
func provisionControlPlane() error {
	// ensure no pre-existing threeport config exists
	if err := os.Remove(threeportConfig); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing test threeport config: %w", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := runTptctlUp(); err != nil {
			errCh <- err
		}
	}()

	if provider == "kind" && imageRepo == "local" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := util.Retry(15, 20, connectLocalRegistry); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	if err, ok := <-errCh; ok && err != nil {
		return fmt.Errorf("failed to run tptctl to provision genesis control plane: %w", err)
	}

	return nil
}

// runTptctlUp provisions a genesis control plane for testing.
func runTptctlUp() error {
	tptctlCmd := filepath.Join(threeportPath, "bin", "tptctl")
	cmdArgs := []string{
		"up",
		"--name",
		threeportName,
		"--provider",
		provider,
		"--control-plane-image-repo",
		getImageRepo(imageRepo),
		"--control-plane-image-tag",
		imageTag,
		"--threeport-config",
		threeportConfig,
	}
	cmd := exec.Command(tptctlCmd, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to provision Threeport control plane with output %s: %w", output, err)
	}

	return nil
}

// connectLocalRegistry configures the kind cluster to use the local registry.
func connectLocalRegistry() error {
	command := "./local-registry.sh"
	args := []string{"connect", fmt.Sprintf("threeport-%s", threeportName)}

	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to connect local container registry to control plane cluster with output %s: %w", output, err)
	}

	return nil
}

// getImageRepo returns the correct image repo for "local" use or the image repo
// specified by user.
func getImageRepo(repo string) string {
	if imageRepo == "local" {
		return "localhost:5001"
	}

	return repo
}

// removeControlPlane uses tptctl to delete the genesis control plane after
// tests are complete.
func removeControlPlane() error {
	tptctlCmd := filepath.Join(threeportPath, "bin", "tptctl")
	cmdArgs := []string{
		"down",
		"--name",
		threeportName,
		"--threeport-config",
		threeportConfig,
	}
	cmd := exec.Command(tptctlCmd, cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to remove Threeport control plane with output %s: %w", output, err)
	}

	return nil
}

// removeLocalRegistry stops and removes the docker container providing the
// local container registry.
func removeLocalRegistry() error {
	if err := tptdev.DeleteLocalRegistry(); err != nil {
		return fmt.Errorf("failed to remove local container registry: %w", err)
	}

	return nil
}
