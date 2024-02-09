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

var createWorkloadInstanceConfigPath string

// CreateWorkloadInstanceCmd represents the workload-instance command
var CreateWorkloadInstanceCmd = &cobra.Command{
	Use:          "workload-instance",
	Example:      "tptctl create workload-instance --config /path/to/config.yaml",
	Short:        "Create a new workload instance",
	Long:         `Create a new workload instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load workload instance config
		configContent, err := os.ReadFile(createWorkloadInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var workloadInstanceConfig config.WorkloadInstanceConfig
		if err := yaml.UnmarshalStrict(configContent, &workloadInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create workload instance
		workloadInstance := workloadInstanceConfig.WorkloadInstance
		wi, err := workloadInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create workload instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload instance %s created\n", *wi.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateWorkloadInstanceCmd)

	CreateWorkloadInstanceCmd.Flags().StringVarP(
		&createWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	CreateWorkloadInstanceCmd.MarkFlagRequired("config")
	CreateWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
