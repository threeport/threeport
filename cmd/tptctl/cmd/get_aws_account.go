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

// GetAwsAccountsCmd represents the aws-accounts command
var GetAwsAccountsCmd = &cobra.Command{
	Use:          "aws-accounts",
	Example:      "tptctl get aws-accounts",
	Short:        "Get AWS accounts from the system",
	Long:         `Get AWS accounts from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

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
				requestedControlPlane,
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
	GetCmd.AddCommand(GetAwsAccountsCmd)
	GetAwsAccountsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
