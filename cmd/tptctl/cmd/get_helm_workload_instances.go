/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GetHelmWorkloadInstancesCmd represents the helm-workload-instances command
var GetHelmWorkloadInstancesCmd = &cobra.Command{
	Use:          "helm-workload-instances",
	Example:      "tptctl get helm-workload-instances",
	Short:        "Get helm workload instances from the system",
	Long:         `Get helm workload instances from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// get helm workload instances
		helmWorkloadInstances, err := client.GetHelmWorkloadInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve helm workload instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*helmWorkloadInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No helm workload instances currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		//fmt.Fprintln(writer, "NAME\t HELM WORKLOAD DEFINITION\t KUBERNETES RUNTIME INSTANCE\t STATUS\t AGE")
		fmt.Fprintln(writer, "NAME\t HELM WORKLOAD DEFINITION\t KUBERNETES RUNTIME INSTANCE\t AGE")
		metadataErr := false
		var helmWorkloadDefErr error
		var kubernetesRuntimeInstErr error
		//var statusErr error
		for _, wi := range *helmWorkloadInstances {
			// get helm workload definition name for instance
			var helmWorkloadDef string
			helmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByID(apiClient, apiEndpoint, *wi.HelmWorkloadDefinitionID)
			if err != nil {
				metadataErr = true
				helmWorkloadDefErr = err
				helmWorkloadDef = "<error>"
			} else {
				helmWorkloadDef = *helmWorkloadDefinition.Name
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
			//// get helm workload status
			//var helmWorkloadInstStatus string
			//helmWorkloadInstStatusDetail := status.GetHelmWorkloadInstanceStatus(apiClient, apiEndpoint, &wi)
			//if helmWorkloadInstStatusDetail.Error != nil {
			//	metadataErr = true
			//	statusErr = helmWorkloadInstStatusDetail.Error
			//	helmWorkloadInstStatus = "<error>"
			//}
			//helmWorkloadInstStatus = string(helmWorkloadInstStatusDetail.Status)
			fmt.Fprintln(
				writer, *wi.Name, "\t", helmWorkloadDef, "\t", kubernetesRuntimeInst, "\t", util.GetAge(wi.CreatedAt),
				//helmWorkloadInstStatus, "\t", util.GetAge(wi.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if helmWorkloadDefErr != nil {
				cli.Error("encountered an error retrieving helm workload definition info", helmWorkloadDefErr)
			}
			if kubernetesRuntimeInstErr != nil {
				cli.Error("encountered an error retrieving kubernetes runtime instance info", kubernetesRuntimeInstErr)
			}
			//if statusErr != nil {
			//	cli.Error("encountered an error retrieving helm workload instance status", statusErr)
			//}
			os.Exit(1)
		}
	},
}

func init() {
	GetCmd.AddCommand(GetHelmWorkloadInstancesCmd)
	GetHelmWorkloadInstancesCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
