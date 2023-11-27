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

// GetDomainNameDefinitionsCmd represents the domain-name-definitions command
var GetDomainNameDefinitionsCmd = &cobra.Command{
	Use:          "domain-name-definitions",
	Example:      "tptctl get domain-name-definitions",
	Short:        "Get domain name definitions from the system",
	Long:         `Get domain name definitions from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

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
