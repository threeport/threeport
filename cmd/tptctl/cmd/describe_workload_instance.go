/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/workload/status"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, err := config.GetThreeportConfig()
		requestedInstance := threeportConfig.GetRequestedInstanceName(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

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
			configContent, err := ioutil.ReadFile(describeWorkloadInstanceConfigPath)
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

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedInstance)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForInstance(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
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
	describeCmd.AddCommand(DescribeWorkloadInstanceCmd)

	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	DescribeWorkloadInstanceCmd.Flags().StringVarP(
		&describeWorkloadInstanceName,
		"name", "n", "", "Name of workload instance.",
	)
}

// validateDescribeControlPlaneFlags validates flag inputs as needed.
func validateDescribeWorkloadInstanceFlags(workloadInstConfigPath, workloadInstName string) error {
	if workloadInstConfigPath == "" && workloadInstName == "" {
		return errors.New("must provide either workload instance name or path to config file")
	}

	if workloadInstConfigPath != "" && workloadInstName != "" {
		return errors.New("workload instance name and path to config file provided - provide only one")
	}

	return nil
}
