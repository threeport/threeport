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

// GetGatewayDefinitionsCmd represents the gateway-definitions command
var GetGatewayDefinitionsCmd = &cobra.Command{
	Use:          "gateway-definitions",
	Example:      "tptctl get gateway-definitions",
	Short:        "Get gateway definitions from the system",
	Long:         `Get gateway definitions from the system.`,
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
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
		}

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
		fmt.Fprintln(writer, "NAME\t TCP PORT\t HTTPS REDIRECT\t AGE")
		for _, g := range *gatewayDefinitions {
			fmt.Fprintln(writer, *g.Name, "\t", *g.TCPPort, "\t", *g.HTTPSRedirect, "\t",
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
