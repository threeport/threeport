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
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		awsRelationalDatabaseInstanceConfig := config.AwsRelationalDatabaseInstanceConfig{
			AwsRelationalDatabaseInstance: config.AwsRelationalDatabaseInstanceValues{
				Name: deleteAwsRelationalDatabaseInstanceName,
			},
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
	DeleteCmd.AddCommand(DeleteAwsRelationalDatabaseInstanceCmd)

	DeleteAwsRelationalDatabaseInstanceCmd.Flags().StringVarP(
		&deleteAwsRelationalDatabaseInstanceName,
		"name", "n", "", "Name of AWS relational database instance.",
	)
	DeleteAwsRelationalDatabaseInstanceCmd.MarkFlagRequired("name")
}
