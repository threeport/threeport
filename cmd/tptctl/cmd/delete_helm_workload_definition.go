/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var (
	deleteHelmWorkloadDefinitionConfigPath string
	deleteHelmWorkloadDefinitionName       string
)

// DeleteHelmWorkloadDefinitionCmd represents the helm-workload-definition command
var DeleteHelmWorkloadDefinitionCmd = &cobra.Command{
	Use:          "helm-workload-definition",
	Example:      "tptctl delete helm-workload-definition --config /path/to/config.yaml",
	Short:        "Delete an existing helm workload definition",
	Long:         `Delete an existing helm workload definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteHelmWorkloadDefinitionConfigPath,
			deleteHelmWorkloadDefinitionName,
			"helm workload definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var helmWorkloadDefinitionConfig config.HelmWorkloadDefinitionConfig
		if deleteHelmWorkloadDefinitionConfigPath != "" {
			// load helm workload definition config
			configContent, err := os.ReadFile(deleteHelmWorkloadDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &helmWorkloadDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			helmWorkloadDefinitionConfig = config.HelmWorkloadDefinitionConfig{
				HelmWorkloadDefinition: config.HelmWorkloadDefinitionValues{
					Name: deleteHelmWorkloadDefinitionName,
				},
			}
		}

		// delete helm workload definition
		helmWorkloadDefinition := helmWorkloadDefinitionConfig.HelmWorkloadDefinition
		_, err := helmWorkloadDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete helm workload definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("helm workload definition %s deleted", helmWorkloadDefinition.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteHelmWorkloadDefinitionCmd)

	DeleteHelmWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteHelmWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with helm workload definition config.",
	)
	DeleteHelmWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteHelmWorkloadDefinitionName,
		"name", "n", "", "Name of helm workload definition.",
	)
	DeleteHelmWorkloadDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
