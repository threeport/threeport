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

var createControlPlaneInstanceConfigPath string

// CreateControlPlaneInstanceCmd represents the workload-instance command
var CreateControlPlaneInstanceCmd = &cobra.Command{
	Use:          "control-plane-instance",
	Example:      "tptctl create control-plane-instance --config /path/to/config.yaml",
	Short:        "Create a new control plane instance",
	Long:         `Create a new control plane instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load workload instance config
		configContent, err := ioutil.ReadFile(createControlPlaneInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var controlPlaneInstanceConfig config.ControlPlaneInstanceConfig
		if err := yaml.Unmarshal(configContent, &controlPlaneInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create control plane instance
		controlPlaneInstance := controlPlaneInstanceConfig.ControlPlaneInstance
		ci, err := controlPlaneInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create control plane instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane instance %s created\n", *ci.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateControlPlaneInstanceCmd)

	CreateControlPlaneInstanceCmd.Flags().StringVarP(
		&createControlPlaneInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	CreateControlPlaneInstanceCmd.MarkFlagRequired("config")
}
