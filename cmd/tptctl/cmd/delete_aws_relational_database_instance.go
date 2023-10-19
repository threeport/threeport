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

var deleteAwsRelationalDatabaseInstanceName string

// DeleteAwsRelationalDatabaseInstanceCmd represents the aws-relational-database-instance command
var DeleteAwsRelationalDatabaseInstanceCmd = &cobra.Command{
	Use:          "aws-relational-database-instance",
	Example:      "tptctl delete aws-relational-database-instance --name some-instance",
	Short:        "Delete an existing AWS relational database instance",
	Long:         `Delete an existing AWS relational database instance.`,
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

		awsRelationalDatabaseInstanceConfig := config.AwsRelationalDatabaseInstanceConfig{
			AwsRelationalDatabaseInstance: config.AwsRelationalDatabaseInstanceValues{
				Name: deleteAwsRelationalDatabaseInstanceName,
			},
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// delete AWS relational database instance
		awsRelationalDatabaseInstance := awsRelationalDatabaseInstanceConfig.AwsRelationalDatabaseInstance
		aa, err := awsRelationalDatabaseInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS relational database instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS relational database instance %s deleted", *aa.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteAwsRelationalDatabaseInstanceCmd)

	DeleteAwsRelationalDatabaseInstanceCmd.Flags().StringVarP(
		&deleteAwsRelationalDatabaseInstanceName,
		"name", "n", "", "Name of AWS relational database instance.",
	)
	DeleteAwsRelationalDatabaseInstanceCmd.MarkFlagRequired("name")
}
