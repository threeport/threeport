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

var deleteAwsRelationalDatabaseDefinitionName string

// DeleteAwsRelationalDatabaseDefinitionCmd represents the aws-relational-database-definition command
var DeleteAwsRelationalDatabaseDefinitionCmd = &cobra.Command{
	Use:          "aws-relational-database-definition",
	Example:      "tptctl delete aws-relational-database-definition --name some-definition",
	Short:        "Delete an existing AWS relational database definition",
	Long:         `Delete an existing AWS relational database definition.`,
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

		awsRelationalDatabaseDefinitionConfig := config.AwsRelationalDatabaseDefinitionConfig{
			AwsRelationalDatabaseDefinition: config.AwsRelationalDatabaseDefinitionValues{
				Name: deleteAwsRelationalDatabaseDefinitionName,
			},
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// delete AWS relational database definition
		awsRelationalDatabaseDefinition := awsRelationalDatabaseDefinitionConfig.AwsRelationalDatabaseDefinition
		aa, err := awsRelationalDatabaseDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS relational database definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS relational database definition %s deleted", *aa.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteAwsRelationalDatabaseDefinitionCmd)

	DeleteAwsRelationalDatabaseDefinitionCmd.Flags().StringVarP(
		&deleteAwsRelationalDatabaseDefinitionName,
		"name", "n", "", "Name of AWS relational database definition.",
	)
	DeleteAwsRelationalDatabaseDefinitionCmd.MarkFlagRequired("name")
}
