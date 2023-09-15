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

// GetWorkloadDefinitionsCmd represents the workload-definitions command
var GetWorkloadDefinitionsCmd = &cobra.Command{
	Use:          "workload-definitions",
	Example:      "tptctl get workload-definitions",
	Short:        "Get workload definitions from the system",
	Long:         `Get workload definitions from the system.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedInstance, err := config.GetThreeportConfig(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
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

		// get workload definitions
		workloadDefinitions, err := client.GetWorkloadDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve workload definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*workloadDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No workload definitions currently managed by %s threeport control plane",
				requestedInstance,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t AGE")
		for _, wd := range *workloadDefinitions {
			fmt.Fprintln(writer, *wd.Name, "\t", util.GetAge(wd.CreatedAt))
		}
		writer.Flush()
	},
}

func init() {
	getCmd.AddCommand(GetWorkloadDefinitionsCmd)
	GetWorkloadDefinitionsCmd.Flags().StringVarP(
		&cliArgs.InstanceName,
		"threeport-instance", "i", "", "Optional. Name of control plane instance. Will default to current instance if not provided.",
	)
}
