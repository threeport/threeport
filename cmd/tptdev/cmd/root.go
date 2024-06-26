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

// GetComponentList returns a list of component names to build
func GetComponentList(componentNames string, allComponents []*v0.ControlPlaneComponent) ([]*v0.ControlPlaneComponent, error) {
	componentList := make([]*v0.ControlPlaneComponent, 0)
	switch {
	case len(componentNames) != 0:
		componentNameList := strings.Split(componentNames, ",")
		for _, name := range componentNameList {
			found := false
			for _, c := range allComponents {
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
		componentList = allComponents
	}
	return componentList, nil
}
