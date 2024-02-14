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
	deleteWorkloadInstanceConfigPath string
	deleteWorkloadInstanceName       string
)

// DeleteWorkloadInstanceCmd represents the workload-instance command
var DeleteWorkloadInstanceCmd = &cobra.Command{
	Use:          "workload-instance",
	Example:      "tptctl delete workload-instance --config /path/to/config.yaml",
	Short:        "Delete an existing workload instance",
	Long:         `Delete an existing workload instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteWorkloadInstanceConfigPath,
			deleteWorkloadInstanceName,
			"workload instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadInstanceConfig config.WorkloadInstanceConfig
		if deleteWorkloadInstanceConfigPath != "" {
			// load workload instance config
			configContent, err := os.ReadFile(deleteWorkloadInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &workloadInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadInstanceConfig = config.WorkloadInstanceConfig{
				WorkloadInstance: config.WorkloadInstanceValues{
					Name: deleteWorkloadInstanceName,
				},
			}
		}

		// delete workload instance
		workloadInstance := workloadInstanceConfig.WorkloadInstance
		_, err := workloadInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload instance %s deleted\n", workloadInstance.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteWorkloadInstanceCmd)

	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&deleteWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&deleteWorkloadInstanceName,
		"name", "n", "", "Name of workload instance.",
	)
	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
