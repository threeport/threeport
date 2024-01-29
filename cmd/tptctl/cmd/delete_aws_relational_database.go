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

var (
	deleteAwsRelationalDatabaseConfigPath string
	deleteAwsRelationalDatabaseName       string
)

// DeleteAwsRelationalDatabaseCmd represents the aws-relational-database command
var DeleteAwsRelationalDatabaseCmd = &cobra.Command{
	Use:     "aws-relational-database",
	Example: "tptctl delete aws-relational-database --config /path/to/config.yaml",
	Short:   "Delete an existing AWS relational database",
	Long: `Delete an existing AWS relational database. This command deletes an existing
AWS relational database definition and AWS relational database instance based on
the AWS relational database config or name.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteAwsRelationalDatabaseConfigPath,
			deleteAwsRelationalDatabaseName,
			"AWS relational database",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var awsRelationalDatabaseConfig config.AwsRelationalDatabaseConfig
		if deleteAwsRelationalDatabaseConfigPath != "" {
			// load AWS relational database definition config
			configContent, err := os.ReadFile(deleteAwsRelationalDatabaseConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &awsRelationalDatabaseConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			awsRelationalDatabaseConfig = config.AwsRelationalDatabaseConfig{
				AwsRelationalDatabase: config.AwsRelationalDatabaseValues{
					Name: deleteAwsRelationalDatabaseName,
				},
			}
		}

		// delete AWS relational database
		cli.Info("deleting AWS relational database (this will take a few minutes)...")
		awsRelationalDatabase := awsRelationalDatabaseConfig.AwsRelationalDatabase
		rd, ri, err := awsRelationalDatabase.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS relational database", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("AWS relational database instance %s deleted", *ri.Name))
		cli.Info(fmt.Sprintf("AWS relational database definition %s deleted", *rd.Name))
		cli.Complete(fmt.Sprintf("AWS relational database %s deleted", awsRelationalDatabaseConfig.AwsRelationalDatabase.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteAwsRelationalDatabaseCmd)

	DeleteAwsRelationalDatabaseCmd.Flags().StringVarP(
		&deleteAwsRelationalDatabaseConfigPath,
		"config", "c", "", "Path to file with AWS relational database config.",
	)
	DeleteAwsRelationalDatabaseCmd.Flags().StringVarP(
		&deleteAwsRelationalDatabaseName,
		"name", "n", "", "Name of AWS relational database.",
	)
}
