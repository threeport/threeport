/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/workload/status"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var (
	describeWorkloadInstanceConfigPath string
	describeWorkloadInstanceName       string
)

// DescribeWorkloadInstanceCmd represents the workload-instances command
var DescribeWorkloadInstanceCmd = &cobra.Command{
	Use:          "workload-instance",
	Example:      "tptctl describe workload-instance",
	Short:        "Describe a workload instance",
	Long:         `Describe a workload instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := validateDeleteWorkloadInstanceFlags(
			describeWorkloadInstanceConfigPath,
			describeWorkloadInstanceName,
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadInstanceConfig config.WorkloadInstanceConfig
		if describeWorkloadInstanceConfigPath != "" {
			// load workload instance config
			configContent, err := os.ReadFile(describeWorkloadInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &workloadInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadInstanceConfig = config.WorkloadInstanceConfig{
				WorkloadInstance: config.WorkloadInstanceValues{
					Name: describeWorkloadInstanceName,
				},
			}
		}

		// describe workload instance
		workloadInstance := workloadInstanceConfig.WorkloadInstance
		workloadStatus, err := workloadInstance.Describe(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to describe workload instance", err)
			os.Exit(1)
		}

		// output describe details
		cli.Info(fmt.Sprintf(
			"Workload Instance Name: %s",
			workloadInstanceConfig.WorkloadInstance.Name,
		))
		cli.Info(fmt.Sprintf(
			"Workload Status: %s",
			workloadStatus.Status,
		))
		if workloadStatus.Reason != "" {
			cli.Info(fmt.Sprintf(
				"Workload Status Reason: %s",
				workloadStatus.Reason,
			))
		}
		if len(workloadStatus.Events) > 0 && workloadStatus.Status != status.WorkloadInstanceStatusHealthy {
			cli.Warning("Failed & Warning Events:")
			writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
			fmt.Fprintln(writer, "TYPE\t REASON\t MESSAGE\t AGE")
			for _, event := range workloadStatus.Events {
				fmt.Fprintln(
					writer, *event.Type, "\t", *event.Reason, "\t", *event.Message, "\t",
					util.GetAge(event.Timestamp),
				)
			}
			writer.Flush()
		}
	},
}

func init() {
	DescribeCmd.AddCommand(DescribeWorkloadInstanceCmd)

	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceName,
		"name", "n", "", "Name of workload instance.",
	)
	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
