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
	config "github.com/threeport/threeport/pkg/config/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GetControlPlaneInstancesCmd represents the control-plane-instances command
var GetControlPlaneInstancesCmd = &cobra.Command{
	Use:          "control-plane-instances",
	Example:      "tptctl get control-plane-instances",
	Short:        "Get control plane instances from the system",
	Long:         `Get control plane instances from the system.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// get control plane instances
		controlPlaneInstances, err := client.GetControlPlaneInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve control plane instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*controlPlaneInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No control plane instances currently managed by %s threeport control plane",
				threeportConfig.CurrentControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t WORKLOAD DEFINITION\t KUBERNETES RUNTIME INSTANCE\t AGE")
		metadataErr := false
		var controlPlaneDefErr error
		var kubernetesRuntimeInstErr error
		for _, ci := range *controlPlaneInstances {
			// get control plane definition name for instance
			var controlPlaneDef string
			controlPlaneDefinition, err := client.GetControlPlaneDefinitionByID(apiClient, apiEndpoint, *ci.ControlPlaneDefinitionID)
			if err != nil {
				metadataErr = true
				controlPlaneDefErr = err
				controlPlaneDef = "<error>"
			} else {
				controlPlaneDef = *controlPlaneDefinition.Name
			}
			// get kubernetes runtime instance name for instance
			var kubernetesRuntimeInst string
			kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, *ci.KubernetesRuntimeInstanceID)
			if err != nil {
				metadataErr = true
				kubernetesRuntimeInstErr = err
				kubernetesRuntimeInst = "<error>"
			} else {
				kubernetesRuntimeInst = *kubernetesRuntimeInstance.Name
			}
			fmt.Fprintln(
				writer, *ci.Name, "\t", controlPlaneDef, "\t", kubernetesRuntimeInst,
				"\t", util.GetAge(ci.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if controlPlaneDefErr != nil {
				cli.Error("encountered an error retrieving control plane definition info", controlPlaneDefErr)
			}
			if kubernetesRuntimeInstErr != nil {
				cli.Error("encountered an error retrieving kubernetes runtime instance info", kubernetesRuntimeInstErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	getCmd.AddCommand(GetControlPlaneInstancesCmd)
}
