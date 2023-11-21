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

// GetControlPlanesCmd represents the get control planes command
var GetControlPlanesCmd = &cobra.Command{
	Use:     "control-planes",
	Example: "tptctl get control-planes",
	Short:   "Get control-planes from the system",
	Long: `Get control-planes from the system.

A control plane is a simple abstraction of control plane definitions and control plane instances.
This command displays all instances and the definitions used to configure them.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, _ := getClientContext(cmd)

		// get control plane instances
		controlPlaneInstances, err := client.GetControlPlaneInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve control plane instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*controlPlaneInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No control planes currently managed by %s threeport control plane",
				threeportConfig.CurrentControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t CONTROL PLANE DEFINITION\t CONTROL PLANE INSTANCE\t KUBERNETES RUNTIME INSTANCE\t AGE")
		metadataErr := false
		var controlPlaneDefinitionErr error
		var kubernetesRuntimeInstErr error
		for _, ci := range *controlPlaneInstances {
			// get control plane definition name for instance
			var controlPlaneDef string
			controlPlaneDefinition, err := client.GetControlPlaneDefinitionByID(apiClient, apiEndpoint, *ci.ControlPlaneDefinitionID)
			if err != nil {
				metadataErr = true
				controlPlaneDefinitionErr = err
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
				writer, controlPlaneDef, "\t", controlPlaneDef, "\t", *ci.Name, "\t", kubernetesRuntimeInst, "\t", util.GetAge(ci.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if controlPlaneDefinitionErr != nil {
				cli.Error("encountered an error retrieving control plane definition info", controlPlaneDefinitionErr)
			}
			if kubernetesRuntimeInstErr != nil {
				cli.Error("encountered an error retrieving kubernetes runtime instance info", kubernetesRuntimeInstErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	GetCmd.AddCommand(GetControlPlanesCmd)
}
