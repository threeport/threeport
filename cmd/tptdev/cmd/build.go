/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

var all bool
var noCache bool
var imageNames string
var arch string
var parallel int

// buildCmd represents the up command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {

		// create list of images to build
		imageNamesList := getImageNamesList(all, imageNames)

		// update cli args based on env vars
		getControlPlaneEnvVars()

		// configure concurrency for parallel builds
		jobs := make(chan string)
		var jobsWaitGroup sync.WaitGroup

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
			jobsWaitGroup.Add(1)
			go func() {
				defer jobsWaitGroup.Done()
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
					if err := tptdev.BuildDockerxImage(
						cpi.Opts.ThreeportPath,
						image,
						tag,
						arch,
					); err != nil {
						cli.Error("failed to build docker image: %v", err)
						os.Exit(1)
					}

					// push docker image
					if err := tptdev.PushDockerImage(tag); err != nil {
						cli.Error("failed to push docker image: %v", err)
						os.Exit(1)
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
		jobsWaitGroup.Wait()
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVar(
		&imageNames,
		"image-names", "", "Image name",
	)
	buildCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageRepo,
		"control-plane-image-repo", "", "Alternate image repo to pull threeport control plane images from.",
	)
	buildCmd.Flags().StringVar(
		&cliArgs.ControlPlaneImageTag,
		"control-plane-image-tag", "", "Alternate image tag to pull threeport control plane images from.",
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
	// buildCmd.Flags().BoolVar(
	// 	&cliArgs.AuthEnabled,
	// 	"auth-enabled", false, "Enable client certificate authentication (default is false).",
	// )
	// buildCmd.Flags().StringVarP(
	// 	&cliArgs.ControlPlaneName,
	// 	"name", "n", tptdev.DefaultInstanceName, "Name of dev control plane instance.",
	// )
	// buildCmd.Flags().StringVarP(
	// 	&cliArgs.ThreeportPath,
	// 	"threeport-path", "t", "", "Path to threeport repository root (default is './').",
	// )
	// rootCmd.PersistentFlags().StringVar(
	// 	&cliArgs.CfgFile,
	// 	"threeport-config", "", "Path to config file (default is $HOME/.config/threeport/config.yaml).",
	// )
	// rootCmd.PersistentFlags().StringVar(
	// 	&cliArgs.ProviderConfigDir,
	// 	"provider-config", "", "Path to infra provider config directory (default is $HOME/.config/threeport/).",
	// )
	// buildCmd.Flags().IntVar(
	// 	&cliArgs.NumWorkerNodes,
	// 	"num-worker-nodes", 0, "Number of additional worker nodes to deploy (default is 0).",
	// )
	// cobra.OnInitialize(func() {
	// 	cli.InitConfig(cliArgs.CfgFile)
	// })
}
