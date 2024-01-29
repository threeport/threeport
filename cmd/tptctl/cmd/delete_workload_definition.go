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
	deleteWorkloadDefinitionConfigPath string
	deleteWorkloadDefinitionName       string
)

// DeleteWorkloadDefinitionCmd represents the workload-definition command
var DeleteWorkloadDefinitionCmd = &cobra.Command{
	Use:          "workload-definition",
	Example:      "tptctl delete workload-definition --config /path/to/config.yaml",
	Short:        "Delete an existing workload definition",
	Long:         `Delete an existing workload definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteWorkloadDefinitionConfigPath,
			deleteWorkloadDefinitionName,
			"workload definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadDefinitionConfig config.WorkloadDefinitionConfig
		if deleteWorkloadDefinitionConfigPath != "" {
			// load workload definition config
			configContent, err := os.ReadFile(deleteWorkloadDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &workloadDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadDefinitionConfig = config.WorkloadDefinitionConfig{
				WorkloadDefinition: config.WorkloadDefinitionValues{
					Name: deleteWorkloadDefinitionName,
				},
			}
		}

		// delete workload definition
		workloadDefinition := workloadDefinitionConfig.WorkloadDefinition
		_, err := workloadDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload definition %s deleted", workloadDefinition.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteWorkloadDefinitionCmd)

	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.",
	)
	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteWorkloadDefinitionName,
		"name", "n", "", "Name of workload definition.",
	)
	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
