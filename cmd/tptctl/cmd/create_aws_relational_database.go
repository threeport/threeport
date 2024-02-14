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

var createAwsRelationalDatabaseConfigPath string

// CreateAwsRelationalDatabaseCmd represents the aws-relational-database command
var CreateAwsRelationalDatabaseCmd = &cobra.Command{
	Use:     "aws-relational-database",
	Example: "tptctl create aws-relational-database --config /path/to/config.yaml",
	Short:   "Create a new AWS relational database",
	Long: `Create a new AWS relational database. This command creates a new AWS relational database definition
and AWS relational database instance based on the AWS relational database config.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load AWS relational database config
		configContent, err := os.ReadFile(createAwsRelationalDatabaseConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsRelationalDatabaseConfig config.AwsRelationalDatabaseConfig
		if err := yaml.UnmarshalStrict(configContent, &awsRelationalDatabaseConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create AWS relational database
		awsRelationalDatabase := awsRelationalDatabaseConfig.AwsRelationalDatabase
		rd, ri, err := awsRelationalDatabase.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create AWS relational database", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("AWS relational database definition %s created", *rd.Name))
		cli.Info(fmt.Sprintf("AWS relational database instance %s created", *ri.Name))
		cli.Complete(fmt.Sprintf("AWS relational database %s created", awsRelationalDatabaseConfig.AwsRelationalDatabase.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateAwsRelationalDatabaseCmd)

	CreateAwsRelationalDatabaseCmd.Flags().StringVarP(
		&createAwsRelationalDatabaseConfigPath,
		"config", "c", "", "Path to file with AWS relational database config.",
	)
	CreateAwsRelationalDatabaseCmd.MarkFlagRequired("config")
}
