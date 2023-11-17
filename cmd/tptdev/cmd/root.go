/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tptdev",
	Short: "Manage threeport development environments",
	Long:  `Manage threeport development environments.`,
}

var cliArgs = &cli.GenesisControlPlaneCLIArgs{}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tptdev.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	cobra.OnInitialize(func() {
		cli.InitConfig(cliArgs.CfgFile)
		cli.InitArgs(cliArgs)

		cliArgs.InfraProvider = "kind"
		cliArgs.DevEnvironment = true
	})
}

// getComponentList returns a list of component names to build
func getComponentList(componentNames string) ([]*v0.ControlPlaneComponent, error) {
	allComponentList := installer.AllControlPlaneComponents()
	componentList := make([]*v0.ControlPlaneComponent, 0)
	switch {
	case len(componentNames) != 0:
		componentNameList := strings.Split(componentNames, ",")
		for _, name := range componentNameList {
			found := false
			for _, c := range allComponentList {
				if c.Name == name {
					if found {
						return componentList, fmt.Errorf("found more then one component info for: %s", name)
					}
					componentList = append(componentList, c)
					found = true
				}
			}

			if !found {
				return componentList, fmt.Errorf("could not find requested component to install: %s", name)
			}
		}
	default:
		componentList = allComponentList
	}
	return componentList, nil
}

// getControlPlaneEnvVars updates cli args based on env vars
func getControlPlaneEnvVars() {
	// get control plane image repo and tag from env vars
	controlPlaneImageRepo := os.Getenv("CONTROL_PLANE_IMAGE_REPO")
	controlPlaneImageTag := os.Getenv("CONTROL_PLANE_IMAGE_TAG")

	// configure control plane image repo via env var if not provided by cli
	if cliArgs.ControlPlaneImageRepo == "" && controlPlaneImageRepo != "" {
		cliArgs.ControlPlaneImageRepo = controlPlaneImageRepo
	}

	// configure control plane image tag via env var if not provided by cli
	if cliArgs.ControlPlaneImageTag == "" && controlPlaneImageTag != "" {
		cliArgs.ControlPlaneImageTag = controlPlaneImageTag
	}
}
