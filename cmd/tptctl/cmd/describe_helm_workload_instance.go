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
	describeHelmWorkloadInstanceConfigPath string
	describeHelmWorkloadInstanceName       string
)

// DescribeHelmWorkloadInstanceCmd represents the helm-workload-instances command
var DescribeHelmWorkloadInstanceCmd = &cobra.Command{
	Use:          "helm-workload-instance",
	Example:      "tptctl describe helm-workload-instance",
	Short:        "Describe a helm workload instance",
	Long:         `Describe a helm workload instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			describeHelmWorkloadInstanceConfigPath,
			describeHelmWorkloadInstanceName,
			"helm workload instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var helmWorkloadInstanceConfig config.HelmWorkloadInstanceConfig
		if describeHelmWorkloadInstanceConfigPath != "" {
			// load helm workload instance config
			configContent, err := os.ReadFile(describeHelmWorkloadInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &helmWorkloadInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			helmWorkloadInstanceConfig = config.HelmWorkloadInstanceConfig{
				HelmWorkloadInstance: config.HelmWorkloadInstanceValues{
					Name: describeHelmWorkloadInstanceName,
				},
			}
		}

		// describe helm workload instance
		helmWorkloadInstance := helmWorkloadInstanceConfig.HelmWorkloadInstance
		helmWorkloadStatus, err := helmWorkloadInstance.Describe(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to describe helm workload instance", err)
			os.Exit(1)
		}

		// output describe details
		cli.Info(fmt.Sprintf(
			"Helm Workload Instance Name: %s",
			helmWorkloadInstanceConfig.HelmWorkloadInstance.Name,
		))
		cli.Info(fmt.Sprintf(
			"Helm Workload Status: %s",
			helmWorkloadStatus.Status,
		))
		if helmWorkloadStatus.Reason != "" {
			cli.Info(fmt.Sprintf(
				"Helm Workload Status Reason: %s",
				helmWorkloadStatus.Reason,
			))
		}
		if len(helmWorkloadStatus.Events) > 0 && helmWorkloadStatus.Status != status.WorkloadInstanceStatusHealthy {
			cli.Warning("Failed & Warning Events:")
			writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
			fmt.Fprintln(writer, "TYPE\t REASON\t MESSAGE\t AGE")
			for _, event := range helmWorkloadStatus.Events {
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
	DescribeCmd.AddCommand(DescribeHelmWorkloadInstanceCmd)

	DescribeHelmWorkloadInstanceCmd.Flags().StringVarP(
		&describeHelmWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with helm workload instance config.",
	)
	DescribeHelmWorkloadInstanceCmd.Flags().StringVarP(
		&describeHelmWorkloadInstanceName,
		"name", "n", "", "Name of helm workload instance.",
	)
	DescribeHelmWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
