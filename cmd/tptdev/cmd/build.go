/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

var imageNames string

// buildCmd represents the up command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Spin up a new threeport development environment",
	Long:  `Spin up a new threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		numWorkers := 1 // Number of goroutines to run in parallel
		jobs := make(chan string)
		output := make(chan string)
		var wg sync.WaitGroup

		imageNamesList := strings.Split(imageNames, ",")
		go outputHandler(output)

		// Start worker goroutines
		for i := 1; i <= numWorkers; i++ {
			wg.Add(1)
			go worker(i, jobs, output, &wg)
		}

		for _, imageName := range imageNamesList {
			jobs <- imageName
		}
		close(jobs) // Close the jobs channel to signal that no more jobs will be added

		// Wait for all workers to finish
		wg.Wait()

	},
}

func worker(id int, jobs <-chan string, output chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	for image := range jobs {
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			// cli.Error("failed to create threeport control plane installer", err)
			output <- fmt.Sprintf("failed to create threeport control plane installer: %v", err)
			continue
		}

		tptdev.BuildImage(
			cpi.Opts.ThreeportPath,
			cliArgs.ControlPlaneImageRepo,
			cliArgs.ControlPlaneImageTag,
			image)
		if err != nil {
			// cli.Error("failed to create threeport control plane", err)
			output <- fmt.Sprintf("failed to create threeport control plane : %v", err)
			continue
		}
	}
}

func outputHandler(output <-chan string) {
	for {
		message, ok := <-output
		if !ok {
			// Channel is closed, so exit the Goroutine
			return
		}
		fmt.Println("Received:", message)
	}
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
