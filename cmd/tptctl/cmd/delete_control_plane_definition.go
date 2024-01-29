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

var (
	deleteControlPlaneDefinitionConfigPath string
	deleteControlPlaneDefinitionName       string
)

// DeleteControlPlaneDefinitionCmd represents the control-plane-definition command
var DeleteControlPlaneDefinitionCmd = &cobra.Command{
	Use:          "control-plane-definition",
	Example:      "tptctl delete control-plane-definition --config /path/to/config.yaml",
	Short:        "Delete an existing control plane definition",
	Long:         `Delete an existing control plane definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteControlPlaneDefinitionConfigPath,
			deleteControlPlaneDefinitionName,
			"control plane definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var controlPlaneDefinitionConfig config.ControlPlaneDefinitionConfig
		if deleteControlPlaneDefinitionConfigPath != "" {
			// load workload definition config
			configContent, err := ioutil.ReadFile(deleteControlPlaneDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &controlPlaneDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			controlPlaneDefinitionConfig = config.ControlPlaneDefinitionConfig{
				ControlPlaneDefinition: config.ControlPlaneDefinitionValues{
					Name: deleteControlPlaneDefinitionName,
				},
			}
		}

		// delete control plane definition
		controlPlaneDefinition := controlPlaneDefinitionConfig.ControlPlaneDefinition
		wd, err := controlPlaneDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete control plane definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane definition %s deleted", *wd.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteControlPlaneDefinitionCmd)

	DeleteControlPlaneDefinitionCmd.Flags().StringVarP(
		&deleteControlPlaneDefinitionConfigPath,
		"config", "c", "", "Path to file with control plane definition config.",
	)
	DeleteControlPlaneDefinitionCmd.Flags().StringVarP(
		&deleteControlPlaneDefinitionName,
		"name", "n", "", "Name of control plane definition.",
	)
}
