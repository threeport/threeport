/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createAwsAccountConfigPath string

// CreateAwsAccountCmd represents the aws-account command
var CreateAwsAccountCmd = &cobra.Command{
	Use:     "aws-account",
	Example: "tptctl create aws-account --config /path/to/config.yaml",
	Short:   "Create a new AWS account in Threeport",
	Long: `Create a new AWS account in Threeport. This does NOT create a new AWS
account with that provider.  It registers an existing AWS account in the Threeport
control plane so that it may be used to manage infrastructure.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load AWS account config
		configContent, err := os.ReadFile(createAwsAccountConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsAccountConfig config.AwsAccountConfig
		if err := yaml.UnmarshalStrict(configContent, &awsAccountConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create AWS account
		awsAccount := awsAccountConfig.AwsAccount
		aa, err := awsAccount.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create aws account", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("aws account %s created\n", *aa.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateAwsAccountCmd)

	CreateAwsAccountCmd.Flags().StringVarP(
		&createAwsAccountConfigPath,
		"config", "c", "", "Path to file with AWS account config.",
	)
	CreateAwsAccountCmd.MarkFlagRequired("config")
	CreateAwsAccountCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
