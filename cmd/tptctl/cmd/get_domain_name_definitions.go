/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GetDomainNameDefinitionsCmd represents the domain-name-definitions command
var GetDomainNameDefinitionsCmd = &cobra.Command{
	Use:          "domain-name-definitions",
	Example:      "tptctl get domain-name-definitions",
	Short:        "Get domain name definitions from the system",
	Long:         `Get domain name definitions from the system.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		var apiClient *http.Client
		var apiEndpoint string

		apiClient, apiEndpoint = checkContext(cmd)
		if apiClient == nil && apiEndpoint != "" {
			apiEndpoint, err = threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
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
			apiClient, err = client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
			if err != nil {
				cli.Error("failed to create threeport API client", err)
				os.Exit(1)
			}
		}

		// get domain name definitions
		domainNameDefinitions, err := client.GetDomainNameDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve domain name definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*domainNameDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No domain name definitions currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t ZONE\t ADMIN EMAIL\t AGE ")
		for _, dn := range *domainNameDefinitions {
			fmt.Fprintln(writer, *dn.Name, "\t", *dn.Zone, "\t", *dn.AdminEmail, "\t", util.GetAge(dn.CreatedAt))
		}
		writer.Flush()
	},
}

func init() {
	GetCmd.AddCommand(GetDomainNameDefinitionsCmd)
	GetDomainNameDefinitionsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
