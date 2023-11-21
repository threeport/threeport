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
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		awsAccountConfig := config.AwsAccountConfig{
			AwsAccount: config.AwsAccountValues{
				Name: deleteAwsAccountName,
			},
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
