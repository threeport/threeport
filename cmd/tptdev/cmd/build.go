/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
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

		components := installer.AllControlPlaneComponents()
		components = append(components, installer.DatabaseMigrator)

		// create list of components to build
		componentList, err := GetComponentList(buildComponentNames, components)

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

		// configure parallel builds
		if parallel == -1 {
			parallel = len(componentList)
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
						_, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
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

					// restart pods
					if (push || load) && restart {
						// create dynamic client and rest mapper
						dynamicKubeClient, mapper, err := client.GetKubeDynamicClientAndMapper(kubeconfigPath)
						if err != nil {
							cli.Error("failed to create dynamic kube client and mapper", err)
							os.Exit(1)
						}

						// delete pod
						if err := kube.DeletePod(
							dynamicKubeClient,
							&mapper,
							component.Name,
							installer.ControlPlaneNamespace,
						); err != nil {
							cli.Error("failed to delete pod", err)
							os.Exit(1)
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

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVar(
		&buildComponentNames,
		"names", "", "List of component names to build (rest-api,agent,workload-controller etc). Defaults to all images.",
	)
	buildCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "r", "", "Alternate image repo to pull threeport control plane images from.",
	)
	buildCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag to pull threeport control plane images from.",
	)
	buildCmd.Flags().StringVar(
		&arch,
		"arch", "amd64", "Which architecture to build images for. Defaults to x86. Options are amd64 and arm64.",
	)
	buildCmd.Flags().IntVar(
		&parallel,
		"parallel", -1, "Number of parallel builds to run. Defaults to number of images specified.",
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
