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

// InstallSdk builds SDK binary and installs in GOPATH.
func InstallSdk() error {
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

// BuildTptdev builds tptdev binary.
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

	fmt.Println("tptdev built and available at bin/tptdev")

	return nil
}

// InstallTptdev installs tptdev binary at /usr/local/bin/.
func InstallTptdev() error {
	if err := BuildTptdev(); err != nil {
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

// BuildTptctl builds tptctl binary.
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

	fmt.Println("tptctl binary built and available at bin/tptctl")

	return nil
}

// InstallTptctl installs tptctl binary at /usr/local/bin/.
func InstallTptctl() error {
	if err := BuildTptctl(); err != nil {
		return fmt.Errorf("failed to build tptctl: %w", err)
	}

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

	fmt.Println("tptctl binary installed and available at /usr/local/bin/tptctl")

	return nil
}

// Generate runs code generation.  It runs threeport-sdk and generates API
// swagger docs.
func Generate() error {
	err := GenerateCode()
	if err != nil {
		return fmt.Errorf("code generation failed: %w", err)
	}

	err = GenerateDocs()
	if err != nil {
		return fmt.Errorf("docs generation failed: %w", err)
	}

	fmt.Println("code generated successfully")

	return nil
}

// GenerateCode generates code with threeport-sdk.
func GenerateCode() error {

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

// GenerateDocs generates API swagger docs.
func GenerateDocs() error {
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

// AutomatedTests runs automated tests.
func AutomatedTests() error {
	runTests := exec.Command(
		"go",
		"test",
		"-v",
		"./...",
		"-count=1",
	)

	output, err := runTests.CombinedOutput()
	if err != nil {
		return fmt.Errorf("test runs failed to run: '%s': %w", output, err)
	}

	fmt.Println("tests ran successfully")

	return nil
}

// TestCommits checks to make sure commit messages follow conventional commits
// format.
func TestCommits() error {
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

// DevUp runs a local development environment.
func DevUp() error {
	devUp := exec.Command(
		"./bin/tptdev",
		"up",
		"--force-overwrite-config",
		"--auth-enabled=false",
	)

	output, err := devUp.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create local dev environment: '%s': %w", output, err)
	}

	fmt.Println("local dev environment ran successfully")

	return nil
}

// DevDown deletes the local development environment.
func DevDown() error {
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

// // DevLogsAPI follows the log output from the local dev API
// func DevLogsAPI() error {
// 	devlogsAPI := exec.Command(
// 		"kubectl",
// 		"logs",
// 		"deploy/threeport-api-server",
// 		"-n",
// 		"threeport-control-plane",
// 	)
// 	output, err := devlogsAPI.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to create API dev logs: '%s': %w", output, err)
// 	}

// 	fmt.Println("API dev logs successfully created")

// 	return nil
// }

// // DevLogsWRK follows the log output from the local dev workload controller
// func DevLogsWRK() error {
// 	devlogsWRK := exec.Command(
// 		"kubectl",
// 		"logs",
// 		"deploy/threeport-workload-controller",
// 		"-n",
// 		"threeport-control-plane",
// 		"-f",
// 	)
// 	output, err := devlogsWRK.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to create local dev workload controller logs: '%s': %w", output, err)
// 	}

// 	fmt.Println("local dev workload controller logs successfully created")

// 	return nil
// }

// // DevLogsGW follows the log output from the local dev gateway controller
// func DevLogsGW() error {
// 	devlogsGW := exec.Command(
// 		"kubectl",
// 		"logs",
// 		"deploy/threeport-gateway-controller",
// 		"-n",
// 		"threeport-control-plane",
// 		"-f",
// 	)
// 	output, err := devlogsGW.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to create local dev gateway controller logs: '%s': %w", output, err)
// 	}

// 	fmt.Println("local dev gateway controller logs successfully created")

// 	return nil
// }

// // DevLogsKR follows the log output from the local dev kubernetes runtime controller
// func DevLogsKR() error {
// 	devlogsKR := exec.Command(
// 		"kubectl",
// 		"logs",
// 		"deploy/threeport-kubernetes-runtime-controller",
// 		"-n",
// 		"threeport-control-plane",
// 		"-f",
// 	)
// 	output, err := devlogsKR.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to create local dev kubernetes runtime controller logs: '%s': %w", output, err)
// 	}

// 	fmt.Println("local dev kubernetes runtime controller logs successfully created")

// 	return nil
// }

// // DevLogsAWS follows the log output from the local dev aws controller
// func DevLogsAWS() error {
// 	devlogsAWS := exec.Command(
// 		"kubectl",
// 		"logs",
// 		"deploy/threeport-aws-controller",
// 		"-n",
// 		"threeport-control-plane",
// 		"-f",
// 	)
// 	output, err := devlogsAWS.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to create local dev AWS controller logs: '%s': %w", output, err)
// 	}

// 	fmt.Println("local dev aws controller logs successfully created")

// 	return nil
// }

// // DevLogsCP follows the log output from the local dev control plane controller
// func DevLogsCP() error {
// 	devlogsCP := exec.Command(
// 		"kubectl",
// 		"logs",
// 		"deploy/threeport-control-plane-controller",
// 		"-n",
// 		"threeport-control-plane",
// 		"-f",
// 	)
// 	output, err := devlogsCP.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to create local dev control pane controller logs: '%s': %w", output, err)
// 	}

// 	fmt.Println("local dev control pane controller logs successfully created")

// 	return nil
// }

// DevLogsAgent follows the log output from the local dev agent
// func DevLogsAgent() error {
// 	devlogsAgent := exec.Command(
// 		"kubectl",
// 		"logs",
// 		"deploy/threeport-agent",
// 		"-n",
// 		"threeport-control-plane",
// 		"-f",
// 		"-c",
// 		"manager",
// 	)
// 	output, err := devlogsAgent.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to create local dev agent logs: '%s': %w", output, err)
// 	}

// 	fmt.Println("local dev agent logs successfully created")

// 	return nil
// }

// DevForwardAPI forwards local port 1323 to the local dev API
func DevForwardAPI() error {
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

// DevForwardCrdb forwards local port 26257 to local dev cockroach database
func DevForwardCrdb() error {
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

// DevForwardNats forwards local port 33993 to the local dev API nats server
func DevForwardNats() error {
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

// // DevPurgeStreams purges all nats streams
// func DevPurgeStreams() error {
// 	devpurgeStreams := exec.Command(
// 		"nats",
// 		"stream",
// 		"ls",
// 		"--names",
// 		"|",
// 		"xargs",
// 		"-I",
// 		"{}",
// 		"nats",
// 		"stream",
// 		"purge",
// 		"{}",
// 		"--force",
// 	)

// 	output, err := devpurgeStreams.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to purge all nats streams: '%s': %w", output, err)
// 	}

// 	fmt.Println("all nats streams successfully purged")

// 	return nil
// }

// // DevUninstallHelm uninstalls all helm releases
// func DevUninstallHelm() error {
// 	devuninstallHelm := exec.Command(
// 		"helm",
// 		"ls",
// 		"-A",
// 		"--short",
// 		"|",
// 		"xargs",
// 		"-I",
// 		"{}",
// 		"helm",
// 		"uninstall",
// 		"--namespace",
// 		"threeport-control-plane",
// 		"{}",
// 	)

// 	output, err := devuninstallHelm.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to uninstall all helm releases: '%s': %w", output, err)
// 	}

// 	fmt.Println("all helm releases successfully uninstalled")

// 	return nil
// }

// // DevDebugAPI starts debugging session for API
// func DevDebugAPI() error {
// 	devdebugAPI := exec.Command(
// 		"dlv",
// 		"debug",
// 		"cmd/rest-api/main.go",
// 		"--",
// 		"-env-file",
// 		"hack/env",
// 		"-auto-migrate",
// 		"-verbose",
// 	)

// 	output, err := devdebugAPI.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to start debugging session for API: '%s': %w", output, err)
// 	}

// 	fmt.Println("debugging session for API successfully started")

// 	return nil
// }

// // DevDebugWRK starts debugging session for workload-controller
// func DevDebugWRK() error {
// 	devdebugWRK := exec.Command(
// 		"dlv",
// 		"debug",
// 		"cmd/workload-controller/main_gen.go",
// 		"--",
// 		"-auth-enabled=false",
// 		"-api-server=localhost:1323",
// 		"-msg-broker-host=localhost",
// 		"-msg-broker-port=4222",
// 	)

// 	output, err := devdebugWRK.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to start debugging session for workload-controller: '%s': %w", output, err)
// 	}

// 	fmt.Println("debugging session for workload-controller successfully started")

// 	return nil
// }

// // DevDebugGateway starts debugging session for gateway controller
// func DevDebugGateway() error {
// 	devdebugGateway := exec.Command(
// 		"dlv",
// 		"debug",
// 		"--build-flags",
// 		"cmd/gateway-controller/main_gen.go",
// 		"--",
// 		"-auth-enabled=false",
// 		"-api-server=localhost:1323",
// 		"-msg-broker-host=localhost",
// 		"-msg-broker-port=4222",
// 	)

// 	output, err := devdebugGateway.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to start debugging session for gateway controller: '%s': %w", output, err)
// 	}

// 	fmt.Println("debugging session for gateway controller successfully started")

// 	return nil
// }

// // DevResetCrdb resets the dev cockroach database
// func DevResetCrdb() error {
// 	devresetCrdb := exec.Command(
// 		"kubectl", "exec", "-it", "-n", "threeport-control-plane", "crdb-0", "--",
// 		"cockroach", "sql", "--host", "localhost", "--insecure", "--database", "threeport_api",
// 		"--execute",
// 		`TRUNCATE attached_object_references,
// 		workload_events,
// 		helm_workload_definitions,
// 		helm_workload_instances,
// 		workload_definitions,
// 		workload_resource_definitions,
// 		workload_instances,
// 		workload_resource_instances,
// 		gateway_http_ports,
// 		gateway_tcp_ports,
// 		gateway_instances,
// 		gateway_definitions,
// 		domain_name_definitions,
// 		domain_name_instances,
// 		metrics_instances,
// 		metrics_definitions,
// 		logging_instances,
// 		logging_definitions,
// 		observability_dashboard_instances,
// 		observability_dashboard_definitions,
// 		observability_stack_instances,
// 		observability_stack_definitions,
// 		secret_instances,
// 		secret_definitions;
// 		set sql_safe_updates = false;
// 		update kubernetes_runtime_instances set gateway_controller_instance_id = NULL;
// 		update kubernetes_runtime_instances set dns_controller_instance_id = NULL;
// 		update kubernetes_runtime_instances set secrets_controller_instance_id = NULL;
// 		set sql_safe_updates = true;
// 		DELETE FROM control_plane_definitions WHERE name != 'dev-0';
// 		DELETE FROM control_plane_instances WHERE name != 'dev-0';
// 		DELETE FROM control_plane_components WHERE name != 'dev-0';`,
// 	)

// 	output, err := devresetCrdb.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to reset dev cockroach database: '%s': %w", output, err)
// 	}

// 	fmt.Println("dev cockroach database successfully reset")

// 	return nil
// }

// // DevQueryCrdb opens a terminal connection to the dev cockroach database #TODO: move to kubectl exec command that uses `cockroach` binary in contianer
// func DevQueryCrdb() error {
// 	devqueryCrdb := exec.Command(
// 		"kubectl",
// 		"exec",
// 		"-it",
// 		"-n",
// 		"threeport-control-plane",
// 		"crdb-0",
// 		"--",
// 		"cockroach",
// 		"sql",
// 		"--host",
// 		"localhost",
// 		"--insecure",
// 		"--database",
// 		"threeport_api",
// 	)

// 	output, err := devqueryCrdb.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to open a terminal connection to the dev cockroach database: '%s': %w", output, err)
// 	}

// 	fmt.Println("successfully opened a terminal connection to the dev cockroach database")

// 	return nil
// }

// // DevSubNats subscribe to all messages from nats server locally   #TODO: move to kubectl exec command that uses `nats` binary in contianer
// func DevSubNats() error {
// 	devsubNats := exec.Command(
// 		"nats",
// 		"sub",
// 		"-s",
// 		"nats://127.0.0.1:4222",
// 		" \">\" ",
// 	)

// 	output, err := devsubNats.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("failed to subscribe to all messages from nats server locally: '%s': %w", output, err)
// 	}

// 	fmt.Println("successfully subscribed to all messages from nats server locally")

// 	return nil
// }
