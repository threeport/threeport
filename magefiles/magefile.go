package main

import (
	"errors"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"

	version "github.com/threeport/threeport/internal/version"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	sdkgen "github.com/threeport/threeport/pkg/sdk/v0/gen"
	gencontroller "github.com/threeport/threeport/pkg/sdk/v0/gen/internalpkg/controller"
	genapi "github.com/threeport/threeport/pkg/sdk/v0/gen/pkg/api"
	genclient "github.com/threeport/threeport/pkg/sdk/v0/gen/pkg/client"
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

// installDir returns the directory `go install` writes binaries to:
// $GOBIN if set, otherwise $GOPATH/bin. build.Default.GOPATH falls back
// to ~/go when $GOPATH is unset, so the result is always non-empty.
func installDir() string {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return gobin
	}
	return filepath.Join(build.Default.GOPATH, "bin")
}

// Sdk builds the threeport-sdk binary.
func (Build) Sdk() error {
	buildSdkCmd := exec.Command(
		"go",
		"build",
		"-o",
		"bin/threeport-sdk",
		"cmd/sdk/main.go",
	)
	output, err := buildSdkCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed for threeport-sdk binary with output: '%s': %w", output, err)
	}

	fmt.Println("threeport-sdk binary built and available at bin/threeport-sdk")

	return nil
}

// Sdk builds the threeport-sdk binary and installs in $GOBIN (or $GOPATH/bin).
func (Install) Sdk() error {
	build := Build{}
	if err := build.Sdk(); err != nil {
		return fmt.Errorf("failed to build threeport-sdk: %w", err)
	}

	outputPath := filepath.Join(installDir(), "threeport-sdk")

	installSdkCmd := exec.Command(
		"cp",
		"./bin/threeport-sdk",
		outputPath,
	)
	output, err := installSdkCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed for threeport-sdk binary with output: '%s': %w", output, err)
	}

	fmt.Printf("threeport-sdk binary installed and available at %s\n", outputPath)

	return nil
}

// Integration runs integration tests against an existing Threeport control
// plane.
func (Test) Integration() error {
	if err := unmetPrerequisites("integration test", controlPlaneConfigProblems()); err != nil {
		return err
	}

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

// Tptdev builds the tptdev binary and installs in $GOBIN (or $GOPATH/bin).
func (Install) Tptdev() error {
	build := Build{}
	if err := build.Tptdev(); err != nil {
		return fmt.Errorf("failed to build tptdev: %w", err)
	}

	outputPath := filepath.Join(installDir(), "tptdev")

	installTptdevCmd := exec.Command(
		"cp",
		"./bin/tptdev",
		outputPath,
	)
	output, err := installTptdevCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed for tptdev with output: '%s': %w", output, err)
	}

	fmt.Printf("tptdev installed and available at %s\n", outputPath)

	return nil
}

// Tptctl builds tptctl binary.
func (Build) Tptctl() error {
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

	fmt.Println("tptctl binary built and available at bin/tptctl")

	return nil
}

// Tptctl builds the tptctl binary and installs in $GOBIN (or $GOPATH/bin).
func (Install) Tptctl() error {
	build := Build{}
	if err := build.Tptctl(); err != nil {
		return fmt.Errorf("failed to build tptctl: %w", err)
	}

	outputPath := filepath.Join(installDir(), "tptctl")

	installTptctlCmd := exec.Command(
		"cp",
		"./bin/tptctl",
		outputPath,
	)
	output, err := installTptctlCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed for tptctl binary with output: '%s': %w", output, err)
	}

	fmt.Printf("tptctl binary installed and available at %s\n", outputPath)

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

	if err := (Dev{}).GenerateFixture(); err != nil {
		return fmt.Errorf("failed to generate the reconciler test fixture: %w", err)
	}

	fmt.Println("code generated successfully")

	return nil
}

// The fixture is a miniature module. The same generators that emit API methods,
// client code, and reconcilers run over it.

const (
	// fixtureRoot is the directory generated fixture source is written into.
	fixtureRoot = "internal/reconcilertest"
	// fixtureModulePath is the import path generated files use.
	fixtureModulePath = "github.com/threeport/threeport/internal/reconcilertest"
	// fixtureObjectName is the fixture instance type that is persisted.
	fixtureObjectName = "ReconcilerTestInstance"
	// fixtureVolatile is the fixture instance type tagged persist false.
	fixtureVolatile = "ReconcilerTestVolatileInstance"
	// fixtureGroupName is the name of the object group passed to the generator.
	fixtureGroupName = "reconciler-test-fixture"
)

// GenerateFixture generates API methods, client library, and reconcilers for
// the reconciler fixture in this tree. Input is built here because the fixture
// has no SDK config or go.mod; a nested go.mod would drop it from go test ./internal/....
func (Dev) GenerateFixture() error {
	// describe the persisted and persist-false fixture objects
	apiObject := &sdkgen.ApiObject{
		PackageName: "v0",
		Version:     "v0",
		TypeName:    fixtureObjectName,
		Reconciler:  true,
	}
	volatileObject := &sdkgen.ApiObject{
		PackageName: "v0",
		Version:     "v0",
		TypeName:    fixtureVolatile,
		Reconciler:  true,
	}
	apiObjects := []*sdkgen.ApiObject{apiObject, volatileObject}

	// configure the generator for the fixture import path
	generator := &sdkgen.Generator{
		Module:     true,
		ModulePath: fixtureModulePath,
		ApiObjectGroups: []sdkgen.ApiObjectGroup{{
			ModelFilename:         "reconciler_test_fixture.go",
			ControllerDomain:      "ReconcilerTest",
			ControllerName:        "reconciler-test-controller",
			ControllerShortName:   "fixture",
			ControllerPackageName: "fixture",
			ApiObjects:            apiObjects,
			ReconciledObjects: []sdkgen.ReconciledObject{
				{Name: fixtureObjectName, Versions: []string{"v0"}},
				{Name: fixtureVolatile, Versions: []string{"v0"}},
			},
			StructTags: map[string]map[string]map[string]string{
				fixtureObjectName: {"Status": {"validate": "optional"}},
				fixtureVolatile:   {"Data": {"validate": "optional", "persist": "false"}},
			},
			FieldTypes: map[string]map[string]string{
				fixtureObjectName: {"Status": "*string"},
				fixtureVolatile:   {"Data": "*string"},
			},
			StructEmbeds: map[string][]string{
				fixtureObjectName: {"Common", "Instance", "Reconciliation"},
				fixtureVolatile:   {"Common", "Instance", "Reconciliation"},
			},
		}},
		VersionedApiObjectCollections: []sdkgen.VersionedApiObjectCollection{{
			Version: "v0",
			VersionedApiObjectGroups: []sdkgen.VersionedApiObjectGroup{{
				Name:       fixtureGroupName,
				ApiObjects: apiObjects,
			}},
		}},
	}

	// set the fixture module name and API namespace
	sdkConfig := &sdk.SdkConfig{
		ModuleName:   "ReconcilerTest",
		ApiNamespace: "reconcilertest.threeport.io",
	}

	// change to the fixture directory so generated files land under it
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to read working directory: %w", err)
	}
	if err := os.Chdir(fixtureRoot); err != nil {
		return fmt.Errorf("failed to change to fixture directory %s: %w", fixtureRoot, err)
	}
	defer os.Chdir(projectRoot)

	// generate API methods, client library, and reconcilers
	for _, generate := range []struct {
		name string
		run  func(*sdkgen.Generator, *sdk.SdkConfig) error
	}{
		{"api object methods", genapi.GenApiObjectMethods},
		{"client library", genclient.GenClientLib},
		{"reconciler operations", gencontroller.GenReconcilerOperations},
		{"reconcilers", gencontroller.GenReconcilers},
	} {
		if err := generate.run(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate fixture %s: %w", generate.name, err)
		}
	}

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
		"--propertyStrategy",
		"pascalcase",
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

// moduleTestPath is the directory a test module is generated into. It is two
// directories deep so a replace of ../.. reaches the repository root.
const moduleTestPath = "test/module"

// moduleTestGoPath is the Go module path of the generated test module. Any
// path other than this project's own path selects the module code path in the SDK.
const moduleTestGoPath = "github.com/threeport/threeport-module-test"

// moduleTestBinary is the generated module's tptctl plugin name, taken from
// ModuleName in the SDK config.
const moduleTestBinary = "test"

// moduleTestInputs are the tracked files left in place when generated output
// is removed, so the SDK re-emits scaffolding it otherwise writes only once.
var moduleTestInputs = []string{"sdk-config.yaml", "README.md"}

// ModuleGen generates a Threeport module and type-checks it. Generated module
// code calls this repository's exported API, so a changed signature fails here.
func (Test) ModuleGen() error {
	// generate the module test
	if err := generateModuleTest(); err != nil {
		return err
	}

	// type-check the generated module, including magefiles that have no main
	if err := util.RunCommandStreamOutputInDir(
		moduleTestPath,
		"go", "vet", "./...",
	); err != nil {
		return fmt.Errorf("failed to type-check the generated module: %w", err)
	}

	// remove generated files after a successful type-check
	if err := resetModuleTestDir(); err != nil {
		return err
	}

	fmt.Println("module generated and type-checked successfully")

	return nil
}

// controlPlaneConfigProblems returns errors when the Threeport config is
// missing or has no API endpoint for the current control plane.
func controlPlaneConfigProblems() []error {
	// read the Threeport config tptctl wrote to disk
	cfgFile := cli.DetermineThreeportConfigPath("")
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		return []error{fmt.Errorf("failed to read the Threeport config: %w", err)}
	}
	// parse the Threeport config
	var threeportConfig cli.ThreeportConfig
	if err := yaml.Unmarshal(data, &threeportConfig); err != nil {
		return []error{fmt.Errorf("failed to parse the Threeport config: %w", err)}
	}

	// require a current control plane
	controlPlaneName := threeportConfig.CurrentControlPlane
	if controlPlaneName == "" {
		return []error{errors.New("current control plane must be set in the Threeport config")}
	}

	// require an API endpoint for the current control plane
	if _, err := threeportConfig.GetThreeportAPIEndpoint(controlPlaneName); err != nil {
		return []error{fmt.Errorf(
			"failed to get the API endpoint for control plane %s: %w",
			controlPlaneName, err,
		)}
	}

	return nil
}

// unmetPrerequisites joins prerequisite errors into one error named for the
// target. It returns nil when there are none.
func unmetPrerequisites(target string, problems []error) error {
	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("%s prerequisites are not met:\n%w", target, errors.Join(problems...))
}

// checkModuleInstallPrerequisites reports missing mage or a missing API
// endpoint for the current control plane, not whether the cluster can pull images.
func checkModuleInstallPrerequisites() error {
	var problems []error

	// require mage on PATH for the build and install targets of the generated module
	if _, err := exec.LookPath("mage"); err != nil {
		problems = append(problems, errors.New("mage is not on PATH"))
	}
	// require an API endpoint for the current control plane
	problems = append(problems, controlPlaneConfigProblems()...)

	return unmetPrerequisites("module install", problems)
}

// ModuleInstall generates a Threeport module, builds its images, and installs
// it into the current control plane. It covers registration and routes that
// compiling cannot.
func (Test) ModuleInstall() error {
	// check module install prerequisites
	if err := checkModuleInstallPrerequisites(); err != nil {
		return err
	}

	// generate the module test
	if err := generateModuleTest(); err != nil {
		return err
	}

	// build and push module images to the development registry
	if err := util.RunCommandStreamOutputInDir(
		moduleTestPath,
		"mage", "build:allImagesDev",
	); err != nil {
		return fmt.Errorf("failed to build the module images: %w", err)
	}

	// install the module plugin where tptctl looks for it
	if err := util.RunCommandStreamOutputInDir(
		moduleTestPath,
		"mage", "install:plugin",
	); err != nil {
		return fmt.Errorf("failed to install the module plugin: %w", err)
	}
	// remove the installed plugin when the target returns
	defer removeModuleTestPlugin()

	// install tptctl from this checkout
	install := Install{}
	if err := install.Tptctl(); err != nil {
		return fmt.Errorf("failed to install tptctl: %w", err)
	}

	// install the module with the same registry and imagePullPolicy Always
	if err := util.RunCommandStreamOutputInDir(
		moduleTestPath,
		filepath.Join(installDir(), "tptctl"),
		moduleTestBinary, "install", "--debug",
		"-r", installer.DevImageNamespace,
	); err != nil {
		return fmt.Errorf("failed to install the module: %w", err)
	}
	// remove generated files after a successful install
	if err := resetModuleTestDir(); err != nil {
		return err
	}

	fmt.Println("module generated, installed and registered successfully")

	return nil
}

// removeModuleTestPlugin deletes the generated module's plugin from the tptctl
// plugin directory so later tptctl runs do not load it.
func removeModuleTestPlugin() error {
	// use THREEPORT_PLUGIN_DIR when set, otherwise the default plugin directory
	pluginDir := os.Getenv("THREEPORT_PLUGIN_DIR")
	if pluginDir == "" {
		dir, err := cli.DefaultPluginDir()
		if err != nil {
			return fmt.Errorf("failed to determine tptctl plugin directory: %w", err)
		}
		pluginDir = dir
	}

	// remove the plugin binary if present
	pluginPath := filepath.Join(pluginDir, moduleTestBinary)
	if err := os.Remove(pluginPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove the module test plugin %s: %w", pluginPath, err)
	}

	return nil
}

// generateModuleTest generates a module under the module test directory from
// the tracked SDK config. It does not compile the result.
func generateModuleTest() error {
	// build threeport-sdk from this checkout rather than using an installed binary
	build := Build{}
	if err := build.Sdk(); err != nil {
		return fmt.Errorf("failed to build threeport-sdk: %w", err)
	}

	// resolve the built binary for commands run in the module directory
	sdkBinary, err := filepath.Abs(filepath.Join("bin", "threeport-sdk"))
	if err != nil {
		return fmt.Errorf("failed to resolve the threeport-sdk binary path: %w", err)
	}

	// remove generated files so scaffolding is re-emitted
	if err := resetModuleTestDir(); err != nil {
		return err
	}

	// initialize a Go module whose path selects the module code path in the SDK
	if err := util.RunCommandStreamOutputInDir(
		moduleTestPath,
		"go", "mod", "init", moduleTestGoPath,
	); err != nil {
		return fmt.Errorf("failed to initialize the module test go.mod: %w", err)
	}
	// point the generated module at this checkout
	if err := util.RunCommandStreamOutputInDir(
		moduleTestPath,
		"go", "mod", "edit",
		"-require=github.com/threeport/threeport@v0.0.0",
		"-replace=github.com/threeport/threeport=../..",
	); err != nil {
		return fmt.Errorf("failed to point the module test at this checkout: %w", err)
	}

	// scaffold the API objects, then generate the boilerplate
	for _, subcommand := range []string{"create", "gen"} {
		if err := util.RunCommandStreamOutputInDir(
			moduleTestPath,
			sdkBinary,
			subcommand,
			"-c",
			"sdk-config.yaml",
		); err != nil {
			return fmt.Errorf("failed to run threeport-sdk %s for the module test: %w", subcommand, err)
		}
	}

	// resolve generated module dependencies
	if err := util.RunCommandStreamOutputInDir(
		moduleTestPath,
		"go", "mod", "tidy",
	); err != nil {
		return fmt.Errorf("failed to resolve the module test dependencies: %w", err)
	}

	return nil
}

// resetModuleTestDir removes generated files from the module test directory,
// leaving the tracked inputs in place.
func resetModuleTestDir() error {
	// list entries in the module test directory
	entries, err := os.ReadDir(moduleTestPath)
	if err != nil {
		return fmt.Errorf("failed to read module test directory %s: %w", moduleTestPath, err)
	}

	// skip tracked inputs and remove generated paths
	for _, entry := range entries {
		if slices.Contains(moduleTestInputs, entry.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(moduleTestPath, entry.Name())); err != nil {
			return fmt.Errorf("failed to remove generated path %s: %w", entry.Name(), err)
		}
	}

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
		installer.DevImageNamespace,
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
