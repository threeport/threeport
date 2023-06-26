/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/util"
	"github.com/threeport/threeport/internal/workload/status"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// GetWorkloadsCmd represents the workloads command
var GetWorkloadsCmd = &cobra.Command{
	Use:     "workloads",
	Example: "tptctl get workloads",
	Short:   "Get workloads from the system",
	Long: `Get workloads from the system.

A workload is a simple abstraction of workload definitions and workload instances.
This command displays all instances and the definitions used to configure them.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled()
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
		}

		// get workload instances
		workloadInstances, err := client.GetWorkloadInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve workload instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*workloadInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No workloads currently managed by %s threeport control plane",
				threeportConfig.CurrentInstance,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t WORKLOAD DEFINITION\t WORKLOAD INSTANCE\t CLUSTER INSTANCE\t STATUS\t AGE")
		metadataErr := false
		var workloadDefErr error
		var clusterInstErr error
		var statusErr error
		for _, wi := range *workloadInstances {
			// get workload definition name for instance
			var workloadDef string
			workloadDefinition, err := client.GetWorkloadDefinitionByID(apiClient, apiEndpoint, *wi.WorkloadDefinitionID)
			if err != nil {
				metadataErr = true
				workloadDefErr = err
				workloadDef = "<error>"
			} else {
				workloadDef = *workloadDefinition.Name
			}
			// get cluster instance name for instance
			var clusterInst string
			clusterInstance, err := client.GetClusterInstanceByID(apiClient, apiEndpoint, *wi.ClusterInstanceID)
			if err != nil {
				metadataErr = true
				clusterInstErr = err
				clusterInst = "<error>"
			} else {
				clusterInst = *clusterInstance.Name
			}
			// get workload status
			var workloadInstStatus string
			workloadInstStatusDetail := status.GetWorkloadInstanceStatus(apiClient, apiEndpoint, &wi)
			if workloadInstStatusDetail.Error != nil {
				metadataErr = true
				statusErr = workloadInstStatusDetail.Error
				workloadInstStatus = "<error>"
			}
			workloadInstStatus = string(workloadInstStatusDetail.Status)
			fmt.Fprintln(
				writer, workloadDef, "\t", workloadDef, "\t", *wi.Name, "\t", clusterInst, "\t",
				workloadInstStatus, "\t", util.GetAge(wi.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if workloadDefErr != nil {
				cli.Error("encountered an error retrieving workload definition info", workloadDefErr)
			}
			if clusterInstErr != nil {
				cli.Error("encountered an error retrieving cluster instance info", clusterInstErr)
			}
			if statusErr != nil {
				cli.Error("encountered an error retrieving workload instance status", statusErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	getCmd.AddCommand(GetWorkloadsCmd)
}
