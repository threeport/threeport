/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createControlPlaneConfigPath string

// CreateControlPlaneCmd represents the create threeport command
var CreateControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl create control-plane --c my-threeport.yaml",
	Short:        "Create a new Threeport control plane",
	Long:         `Create a new control plane.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load control plane config
		configContent, err := ioutil.ReadFile(createControlPlaneConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var controlPlaneConfig config.ControlPlaneConfig
		if err := yaml.Unmarshal(configContent, &controlPlaneConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}
		// create control plane
		controlPlane := controlPlaneConfig.ControlPlane
		cd, ci, err := controlPlane.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create control plane", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("control plane definition %s created", *cd.Name))
		cli.Info(fmt.Sprintf("control plane instance %s created", *ci.Name))
		cli.Complete(fmt.Sprintf("control plane %s created", controlPlane.Name))

	},
}

func init() {
	CreateCmd.AddCommand(CreateControlPlaneCmd)
	CreateControlPlaneCmd.Flags().StringVarP(
		&createControlPlaneConfigPath,
		"config", "c", "", "Path to file with control plane config.",
	)
	CreateControlPlaneCmd.MarkFlagRequired("config")
}
