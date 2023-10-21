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

var createAwsRelationalDatabaseInstanceConfigPath string

// CreateAwsRelationalDatabaseInstanceCmd represents the aws-relational-database-instance command
var CreateAwsRelationalDatabaseInstanceCmd = &cobra.Command{
	Use:          "aws-relational-database-instance",
	Example:      "tptctl create aws-relational-database-instance --config /path/to/config.yaml",
	Short:        "Create a new AWS relational database instance",
	Long:         `Create a new AWS relational database instance.`,
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

		// load AWS relational database instance config
		configContent, err := os.ReadFile(createAwsRelationalDatabaseInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsRelationalDatabaseInstanceConfig config.AwsRelationalDatabaseInstanceConfig
		if err := yaml.Unmarshal(configContent, &awsRelationalDatabaseInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// create AWS relational database instance
		awsRelationalDatabaseInstance := awsRelationalDatabaseInstanceConfig.AwsRelationalDatabaseInstance
		kri, err := awsRelationalDatabaseInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create AWS relational database instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS relational database instance %s created\n", *kri.Name))
	},
}

func init() {
	createCmd.AddCommand(CreateAwsRelationalDatabaseInstanceCmd)

	CreateAwsRelationalDatabaseInstanceCmd.Flags().StringVarP(
		&createAwsRelationalDatabaseInstanceConfigPath,
		"config", "c", "", "Path to file with AWS relational database instance config.",
	)
	CreateAwsRelationalDatabaseInstanceCmd.MarkFlagRequired("config")
}
