/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var deleteAwsAccountName string

// DeleteAwsAccountCmd represents the aws-account command
var DeleteAwsAccountCmd = &cobra.Command{
	Use:          "aws-account",
	Example:      "tptctl delete aws-account --name some-account",
	Short:        "Delete an existing AWS account",
	Long:         `Delete an existing AWS account.`,
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

		awsAccountConfig := config.AwsAccountConfig{
			AwsAccount: config.AwsAccountValues{
				Name: deleteAwsAccountName,
			},
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// delete AWS account
		awsAccount := awsAccountConfig.AwsAccount
		aa, err := awsAccount.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS account", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS account %s deleted", *aa.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteAwsAccountCmd)

	DeleteAwsAccountCmd.Flags().StringVarP(
		&deleteAwsAccountName,
		"name", "n", "", "Name of AWS account.",
	)
	DeleteAwsAccountCmd.MarkFlagRequired("name")
	DeleteAwsAccountCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
