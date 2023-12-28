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

// GetGatewayDefinitionsCmd represents the gateway-definitions command
var GetGatewayDefinitionsCmd = &cobra.Command{
	Use:          "gateway-definitions",
	Example:      "tptctl get gateway-definitions",
	Short:        "Get gateway definitions from the system",
	Long:         `Get gateway definitions from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// get gateway definitions
		gatewayDefinitions, err := client.GetGatewayDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve gateway definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*gatewayDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No gateway definitions currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t PORTS\t AGE")
		for _, g := range *gatewayDefinitions {
			// get gateway ports
			gatewayPorts, err := client.GetGatewayPortsAsString(apiClient, apiEndpoint, *g.Common.ID)
			if err != nil {
				cli.Error("failed to get gateway ports as string", err)
				os.Exit(1)
			}
			fmt.Fprintln(
				writer, *g.Name, "\t",
				gatewayPorts, "\t",
				util.GetAge(g.CreatedAt))
		}
		writer.Flush()
	},
}

func init() {
	GetCmd.AddCommand(GetGatewayDefinitionsCmd)
	GetGatewayDefinitionsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
