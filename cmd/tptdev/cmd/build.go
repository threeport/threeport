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
	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

var all bool
var noCache bool
var push bool
var load bool
var imageNames string
var arch string
var parallel int

// buildCmd represents the up command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build threeport docker images.",
	Long:  `Build threeport docker images. Useful for development and debugging. Only supports pushing to Dockerhub and loading into kind.`,
	Run: func(cmd *cobra.Command, args []string) {

		// validate cli args
		if push && load {
			errors.New("cannot use --push and --load together")
			os.Exit(1)
		}

		// create list of images to build
		imageNamesList := getImageNamesList(all, imageNames)

		// update cli args based on env vars
		getControlPlaneEnvVars()

		// configure concurrency for parallel builds
		jobs := make(chan string)
		var waitGroup sync.WaitGroup

		// configure installer
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
		}

		// configure parallel builds
		if parallel == -1 {
			parallel = len(imageNamesList)
		}

		// start build workers
		for i := 1; i <= parallel; i++ {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				for image := range jobs {

					// build go binary
					if err := tptdev.BuildGoBinary(
						cpi.Opts.ThreeportPath,
						image,
						arch,
						noCache,
					); err != nil {
						cli.Error("failed to build go binary: %v", err)
						os.Exit(1)
					}

					// configure image tag
					tag := fmt.Sprintf(
						"%s/threeport-%s:%s",
						cliArgs.ControlPlaneImageRepo,
						image,
						cliArgs.ControlPlaneImageTag,
					)

					// build docker image
					if err := tptdev.DockerBuildxImage(
						cpi.Opts.ThreeportPath,
						image,
						tag,
						arch,
					); err != nil {
						cli.Error("failed to build docker image: %v", err)
						os.Exit(1)
					}

					switch {
					case push:
						// push docker image
						if err := tptdev.PushDockerImage(tag); err != nil {
							cli.Error("failed to push docker image: %v", err)
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
							cli.Error("failed to load docker image into kind: %v", err)
							os.Exit(1)
						}
					}
				}
			}()
		}

		// assign build jobs to workers
		for _, imageName := range imageNamesList {
			jobs <- imageName
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
		&imageNames,
		"image-names", "", "Image name",
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
		&all,
		"all", false, "Alternate image tag to pull threeport control plane images from.",
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
}
