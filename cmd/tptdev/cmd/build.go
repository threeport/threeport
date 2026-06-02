/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var noCache bool
var push bool
var load bool
var buildComponentNames string
var arch string
var parallel int
var restart bool

// buildCmd represents the up command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build threeport docker images.",
	Long:  `Build threeport docker images. Useful for development and debugging. Only supports pushing to Dockerhub and loading into kind.`,
	Run: func(cmd *cobra.Command, args []string) {

		// validate flags
		if push && load {
			cli.Error("error: %w", errors.New("cannot use --push and --load together"))
			os.Exit(1)
		}

		if push && cliArgs.ControlPlaneImageRepo == "" {
			cli.Error("error: %w", errors.New("--control-plane-image-namespace/-r is required when --push is specified"))
			os.Exit(1)
		}

		// default tag to current git branch name if not specified
		if cliArgs.ControlPlaneImageTag == "" {
			branch, err := gitBranchName()
			if err != nil {
				cli.Error(fmt.Sprintf("failed to determine git branch for default tag: %s\nspecify a tag explicitly with --tag/-t", err), nil)
				os.Exit(1)
			}
			cliArgs.ControlPlaneImageTag = branch
		}

		// create list of buildable components, including database-migrator
		componentList, err := GetComponentList(
			buildComponentNames,
			append(installer.AllControlPlaneComponents(), installer.DatabaseMigrator),
		)

		if err != nil {
			cli.Error("failed to get component list:", err)
		}

		// update cli args based on env vars
		cliArgs.GetControlPlaneEnvVars()

		// configure concurrency for parallel builds
		jobs := make(chan *v0.ControlPlaneComponent)
		var waitGroup sync.WaitGroup

		// configure installer
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
		}

		// start build workers
		for i := 1; i <= parallel; i++ {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				for component := range jobs {
					// build go binary
					if err := tptdev.BuildGoBinary(
						cpi.Opts.ThreeportPath,
						arch,
						component,
						noCache,
					); err != nil {
						cli.Error("failed to build go binary:", err)
						os.Exit(1)
					}

					// configure image tag
					tag := fmt.Sprintf(
						"%s/%s:%s",
						cliArgs.ControlPlaneImageRepo,
						component.ImageName,
						cliArgs.ControlPlaneImageTag,
					)

					// build docker image
					if push || load {
						if err := tptdev.DockerBuildxImage(
							cpi.Opts.ThreeportPath,
							"cmd/tptdev/image/Dockerfile",
							tag,
							arch,
							component,
						); err != nil {
							cli.Error("failed to build docker image:", err)
							os.Exit(1)
						}
					}

					switch {
					case push:
						// push docker image
						if err := tptdev.PushDockerImage(tag); err != nil {
							cli.Error("failed to push docker image:", err)
							os.Exit(1)
						}
					case load:
						// get threeport config and extract threeport API endpoint
						_, requestedControlPlane, err := cli.GetThreeportConfig(cliArgs.ControlPlaneName)
						if err != nil {
							cli.Error("failed to get threeport config", err)
							os.Exit(1)
						}

						// load docker image into kind
						if err = tptdev.LoadDevImage(provider.ThreeportRuntimeName(requestedControlPlane), tag); err != nil {
							cli.Error("failed to load docker image into kind:", err)
							os.Exit(1)
						}
					}

					// restart pods with debug mode enabled
					if (push || load) && restart {
						// create dynamic client and rest mapper
						dynamicKubeClient, mapper, err := client_lib.GetKubeDynamicClientAndMapper(kubeconfigPath)
						if err != nil {
							cli.Error("failed to create dynamic kube client and mapper", err)
							os.Exit(1)
						}

						// detect auth state from the running API server deployment
						authEnabled := detectAuthEnabled(dynamicKubeClient)

						cpi.Opts.CreateOrUpdateKubeResources = true
						cpi.Opts.Debug = true
						cpi.Opts.AuthEnabled = authEnabled
						cpi.Opts.Namespace = installer.ControlPlaneNamespace

						switch component.Name {
						case "rest-api":
							if err := cpi.UpdateThreeportAPIDeployment(
								dynamicKubeClient,
								&mapper,
								nil,
							); err != nil {
								cli.Error("failed to update rest-api deployment", err)
								os.Exit(1)
							}
						case "agent":
							if err := cpi.UpdateThreeportAgentDeployment(
								dynamicKubeClient,
								&mapper,
							); err != nil {
								cli.Error("failed to update agent deployment", err)
								os.Exit(1)
							}
						default:
							if err := cpi.UpdateControllerDeployment(
								dynamicKubeClient,
								&mapper,
								*component,
							); err != nil {
								cli.Error("failed to update controller deployment", err)
								os.Exit(1)
							}
						}
					}
				}
			}()
		}

		// assign build jobs to workers
		for _, component := range componentList {
			jobs <- component
		}

		// close the jobs channel to signal that no more jobs will be added
		close(jobs)

		// wait for all workers to finish
		waitGroup.Wait()
	},
}

// gitBranchName returns the current git branch name by reading .git/HEAD directly.
func gitBranchName() (string, error) {
	// walk up from current directory to find .git/HEAD
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	for {
		headPath := filepath.Join(dir, ".git", "HEAD")
		data, err := os.ReadFile(headPath)
		if err == nil {
			ref := strings.TrimSpace(string(data))
			// .git/HEAD contains "ref: refs/heads/<branch>"
			if strings.HasPrefix(ref, "ref: refs/heads/") {
				return strings.TrimPrefix(ref, "ref: refs/heads/"), nil
			}
			return "", fmt.Errorf("git HEAD is detached or has unexpected format: %s", ref)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("not inside a git repository")
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVar(
		&buildComponentNames,
		"names", "", "List of component names to build (rest-api,agent,kubernetes-workload-controller etc). Defaults to all images.",
	)
	buildCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-namespace", "r", "", "Alternate image namespace to pull threeport control plane images from.",
	)
	buildCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag for threeport control plane images.",
	)
	buildCmd.Flags().StringVar(
		&arch,
		"arch", runtime.GOARCH, "Which architecture to build images for. Defaults to the current machine's architecture.",
	)
	buildCmd.Flags().IntVar(
		&parallel,
		"parallel", 1, "Number of parallel builds to run.",
	)
	buildCmd.Flags().BoolVar(
		&noCache,
		"no-cache", false, "Build go binaries without the local go cache.",
	)
	buildCmd.Flags().BoolVar(
		&push,
		"push", false, "Push docker images.",
	)
	buildCmd.Flags().BoolVar(
		&load,
		"load", false, "Load docker images into kind.",
	)
	buildCmd.Flags().BoolVar(
		&restart,
		"restart", false, "Restart pods after pushing or loading images.",
	)
}

// detectAuthEnabled checks the running API server deployment to determine if
// auth is enabled. Returns true (auth enabled) if the deployment doesn't have
// -auth-enabled=false in its args, or if the deployment can't be read.
func detectAuthEnabled(kubeClient *dynamic.DynamicClient) bool {
	deployRes := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploy, err := kubeClient.Resource(deployRes).Namespace(installer.ControlPlaneNamespace).Get(
		context.Background(),
		"threeport-api-server",
		metav1.GetOptions{},
	)
	if err != nil {
		// if we can't read the deployment, default to auth enabled (safe default)
		return true
	}

	containers, found, err := unstructured.NestedSlice(deploy.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		return true
	}

	container := containers[0].(map[string]interface{})
	args, found, err := unstructured.NestedStringSlice(container, "args")
	if err != nil || !found {
		return true
	}

	for _, arg := range args {
		if arg == "-auth-enabled=false" {
			return false
		}
	}

	return true
}
