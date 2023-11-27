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

var createControlPlaneDefinitionConfigPath string

// CreateControlPlaneDefinitionCmd represents the workload-definition command
var CreateControlPlaneDefinitionCmd = &cobra.Command{
	Use:          "control-plane-definition",
	Example:      "tptctl create control-plane-definition --config /path/to/config.yaml",
	Short:        "Create a new control-plane definition",
	Long:         `Create a new control-plane definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load workload definition config
		configContent, err := ioutil.ReadFile(createControlPlaneDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var controlPlaneDefinitionConfig config.ControlPlaneDefinitionConfig
		if err := yaml.Unmarshal(configContent, &controlPlaneDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create workload definition
		controlPlaneDefinition := controlPlaneDefinitionConfig.ControlPlaneDefinition
		wd, err := controlPlaneDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create control plane definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane definition %s created", *wd.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateControlPlaneDefinitionCmd)

	CreateControlPlaneDefinitionCmd.Flags().StringVarP(
		&createControlPlaneDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.",
	)
	CreateControlPlaneDefinitionCmd.MarkFlagRequired("config")
}
