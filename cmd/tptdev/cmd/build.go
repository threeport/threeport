/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	v0 "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

var imageNames string
var parallel int
var all bool
var arch string

// buildCmd represents the up command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {

		// create list of images to build
		imageNamesList := []string{}
		switch all {
		case true:
			for _, controller := range v0.ThreeportControllerList {
				imageNamesList = append(imageNamesList, controller.Name)
			}
			imageNamesList = append(imageNamesList, "rest-api")
			imageNamesList = append(imageNamesList, "agent")
		case false:
			imageNamesList = strings.Split(imageNames, ",")
		}

		// configure concurrency for parallel builds
		jobs := make(chan string)
		output := make(chan string)
		var wg sync.WaitGroup

		// configure output handler
		go func() {
			for {
				message, ok := <-output
				if !ok {
					// Channel is closed, so exit the Goroutine
					return
				}
				fmt.Println("Received:", message)
			}
		}()

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
			wg.Add(1)
			go func() {
				defer wg.Done()
				for image := range jobs {

					// build go binary
					if err := tptdev.BuildGoBinary(
						cpi.Opts.ThreeportPath,
						image,
						arch,
					); err != nil {
						output <- fmt.Sprintf("failed to build go binary: %v", err)
						continue
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
						output <- fmt.Sprintf("failed to build docker image: %v", err)
						continue
					}

					// push docker image
					if err := tptdev.PushDockerImage(tag); err != nil {
						output <- fmt.Sprintf("failed to push docker image: %v", err)
						continue
					}
				}
			}()
		}

		// assign build jobs to workers
		for _, imageName := range imageNamesList {
			jobs <- imageName
		}
		close(jobs) // Close the jobs channel to signal that no more jobs will be added

		// Wait for all workers to finish
		wg.Wait()

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
