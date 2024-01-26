/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/agent"
	"github.com/threeport/threeport/internal/workload/status"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
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
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)
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
				requestedControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t WORKLOAD DEFINITION\t WORKLOAD INSTANCE\t KUBERNETES RUNTIME INSTANCE\t STATUS\t AGE")
		metadataErr := false
		var workloadDefErr error
		var kubernetesRuntimeInstErr error
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
			// get kubernetes runtime instance name for instance
			var kubernetesRuntimeInst string
			kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, *wi.KubernetesRuntimeInstanceID)
			if err != nil {
				metadataErr = true
				kubernetesRuntimeInstErr = err
				kubernetesRuntimeInst = "<error>"
			} else {
				kubernetesRuntimeInst = *kubernetesRuntimeInstance.Name
			}
			// get workload status
			var workloadInstStatus string
			workloadInstStatusDetail := status.GetWorkloadInstanceStatus(
				apiClient,
				apiEndpoint,
				//&wi,
				agent.WorkloadInstanceType,
				*wi.ID,
				*wi.Reconciled,
			)
			if workloadInstStatusDetail.Error != nil {
				metadataErr = true
				statusErr = workloadInstStatusDetail.Error
				workloadInstStatus = "<error>"
			}
			workloadInstStatus = string(workloadInstStatusDetail.Status)
			fmt.Fprintln(
				writer, workloadDef, "\t", workloadDef, "\t", *wi.Name, "\t", kubernetesRuntimeInst, "\t",
				workloadInstStatus, "\t", util.GetAge(wi.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if workloadDefErr != nil {
				cli.Error("encountered an error retrieving workload definition info", workloadDefErr)
			}
			if kubernetesRuntimeInstErr != nil {
				cli.Error("encountered an error retrieving kubernetes runtime instance info", kubernetesRuntimeInstErr)
			}
			if statusErr != nil {
				cli.Error("encountered an error retrieving workload instance status", statusErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	GetCmd.AddCommand(GetWorkloadsCmd)
	GetWorkloadsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
