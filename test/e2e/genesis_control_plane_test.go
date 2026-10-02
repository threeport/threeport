package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	provider_lib "github.com/threeport/threeport/internal/provider"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	imageTag = "test"
)

// setup operations
var _ = BeforeSuite(func() {
	GinkgoWriter.Write([]byte("beginning setup for test suite"))

	By("building tptctl and tptdev")
	err := buildCli()
	Expect(err).NotTo(HaveOccurred(), "failed to build tptctl and tptdev")

	if usingLocalRegistry() {
		By("creating the local container registry")
		err = createLocalRegistry()
		Expect(err).NotTo(HaveOccurred(), "failed to create local container registry")
	}

	if buildingImages() {
		By("building the container images")
		err = buildContainerImages()
		Expect(err).NotTo(HaveOccurred(), "failed to build and push container images")
	}

	if provider == eksProvider {
		By("recording the AWS resources present before the install")
		err = recordAwsResourcesBefore()
		Expect(err).NotTo(HaveOccurred(), "failed to record AWS resources before install")
	}

	By("provisioning genesis control plane")
	GinkgoWriter.Write([]byte("provisioning control plane..."))
	err = provisionControlPlane()
	Expect(err).NotTo(HaveOccurred(), "failed to provision genesis control plane")

	if provider == eksProvider {
		By("checking IAM roles for service accounts works on the cluster")
		err = verifyIrsaOnControlPlaneCluster()
		Expect(err).NotTo(HaveOccurred(), "IRSA is not working on the provisioned cluster")
	}
})

// cleanup operations
var _ = AfterSuite(func() {
	if clean {
		GinkgoWriter.Write([]byte("beginning cleanup for test suite"))

		By("remove genesis control plane")
		err := removeControlPlane()
		Expect(err).NotTo(HaveOccurred(), "failed to remove control plane")

		if usingLocalRegistry() {
			By("remove the local container registry")
			err = removeLocalRegistry()
			Expect(err).NotTo(HaveOccurred(), "failed to remove local container registry")
		}

		if provider == eksProvider {
			// A teardown that reports success while leaving a NAT gateway or a
			// load balancer behind bills by the hour and shows up on nobody's
			// screen, so the suite fails on anything the install added and did
			// not remove.
			By("checking the teardown left no AWS resources behind")
			leaked, err := leakedAwsResourcesAfterTeardown()
			Expect(err).NotTo(HaveOccurred(), "failed to check for leaked AWS resources")
			Expect(leaked).To(
				BeEmpty(),
				"the teardown left AWS resources behind:\n  %s",
				strings.Join(leaked, "\n  "),
			)
		}
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
						"\nTest case name: %s\nTest case object: %s\nTest case config file path: %s\nTest case deployment name: %s\nTest case expected to work: %t\nError: %v\n",
						testCase.Name,
						testCase.Object,
						testCase.ConfigPath,
						testCase.DeploymentName,
						testCase.ShouldWork,
						err,
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
				if testCase.ShouldWork && testCase.Object == "kubernetes-workload-definition" {
					err := testCase.DeleteDefinitions(threeportPath)
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
				if testCase.ShouldWork && testCase.Object != "kubernetes-workload-definition" {
					err := testCase.DeleteInstances(threeportPath)
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
				if testCase.ShouldWork && testCase.Object == "kubernetes-workload-definition" {
					err := testCase.DeleteDefinitions(threeportPath)
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

	// One slot per goroutine started below.  With an unbuffered channel a
	// goroutine that fails blocks forever on the send - nothing reads the
	// channel until after wg.Wait(), and its deferred wg.Done() never runs -
	// so the suite hangs instead of reporting why provisioning failed.
	errCh := make(chan error, 2)

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

	var provisionErrors []error
	for err := range errCh {
		if err != nil {
			provisionErrors = append(provisionErrors, err)
		}
	}
	if len(provisionErrors) > 0 {
		return fmt.Errorf(
			"failed to run tptctl to provision genesis control plane: %w",
			errors.Join(provisionErrors...),
		)
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
		"--threeport-config",
		threeportConfig,
	}
	if buildingImages() {
		cmdArgs = append(
			cmdArgs,
			"--control-plane-image-namespace", getImageRepo(imageRepo),
			"--control-plane-image-tag", imageTag,
		)
	}
	if provider == "kind" && apiPort != 0 {
		cmdArgs = append(cmdArgs, "--api-port", strconv.Itoa(apiPort))
	}
	if provider == eksProvider {
		cmdArgs = append(
			cmdArgs,
			"--aws-region", awsRegion,
			"--aws-config-profile", awsConfigProfile,
			// a failure part way through otherwise leaves a half-built cluster
			// for somebody to find later
			"--teardown-on-failure",
		)
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
		// without this the command waits on a confirmation that never comes,
		// prints "Aborted." and tears nothing down
		"--yes",
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

// eksProvider is the infrastructure provider name for AWS EKS.
const eksProvider = "eks"

// awsResourcesBeforeInstall is what the account held before the suite
// provisioned anything, so that the teardown can be held to putting it back.
var awsResourcesBeforeInstall tpaws.AwsResourceInventory

// usingLocalRegistry reports whether the suite runs its own container
// registry.  Only a local cluster can pull from one.
func usingLocalRegistry() bool {
	return imageRepo == "local" && provider == "kind"
}

// buildingImages reports whether the suite builds the control plane images.
// With no repo given it installs the released images instead, which is what a
// remote provider needs unless its nodes can reach a repo you control.
func buildingImages() bool {
	return imageRepo != ""
}

// e2eAwsConfig returns the AWS config the suite inspects the account with.
func e2eAwsConfig() (aws.Config, error) {
	awsConfig, err := tpaws.LoadAwsConfig(awsConfigProfile, awsRegion)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return *awsConfig, nil
}

// recordAwsResourcesBefore records the AWS resources present before the
// install.
func recordAwsResourcesBefore() error {
	awsConfig, err := e2eAwsConfig()
	if err != nil {
		return err
	}

	inventory, err := tpaws.GetAwsResourceInventory(context.Background(), awsConfig)
	if err != nil {
		return err
	}
	awsResourcesBeforeInstall = inventory

	return nil
}

// leakedAwsResourcesAfterTeardown returns the AWS resources the install added
// and the teardown did not remove.
func leakedAwsResourcesAfterTeardown() ([]string, error) {
	awsConfig, err := e2eAwsConfig()
	if err != nil {
		return nil, err
	}

	inventory, err := tpaws.GetAwsResourceInventory(context.Background(), awsConfig)
	if err != nil {
		return nil, err
	}

	return tpaws.LeakedResources(awsResourcesBeforeInstall, inventory), nil
}

// verifyIrsaOnControlPlaneCluster checks the provisioned cluster can give its
// add-ons AWS credentials through IAM roles for service accounts.
func verifyIrsaOnControlPlaneCluster() error {
	awsConfig, err := e2eAwsConfig()
	if err != nil {
		return err
	}

	return verifyEksIrsa(
		context.Background(),
		awsConfig,
		provider_lib.ThreeportRuntimeName(threeportName),
	)
}
