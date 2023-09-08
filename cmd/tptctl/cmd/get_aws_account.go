/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	"github.com/threeport/threeport/pkg/util/v0"
)

// GetAwsAccountsCmd represents the aws-accounts command
var GetAwsAccountsCmd = &cobra.Command{
	Use:          "aws-accounts",
	Example:      "tptctl get aws-accounts",
	Short:        "Get AWS accounts from the system",
	Long:         `Get AWS accounts from the system.`,
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

		// get AWS accounts
		awsAccounts, err := client.GetAwsAccounts(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve AWS accounts", err)
			os.Exit(1)
		}

		// write the output
		if len(*awsAccounts) == 0 {
			cli.Info(fmt.Sprintf(
				"No AWS accounts currently managed by %s threeport control plane",
				threeportConfig.CurrentInstance,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t DEFAULT ACCOUNT\t DEFAULT REGION\t ACCOUNT ID\t AGE")
		for _, aa := range *awsAccounts {
			fmt.Fprintln(writer, *aa.Name, "\t", *aa.DefaultAccount, "\t", *aa.DefaultRegion, "\t", *aa.AccountID, "\t", util.GetAge(aa.CreatedAt))
		}
		writer.Flush()
	},
}

func init() {
	getCmd.AddCommand(GetAwsAccountsCmd)
}
