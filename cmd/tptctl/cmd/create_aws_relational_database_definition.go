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

var createAwsRelationalDatabaseDefinitionConfigPath string

// CreateAwsRelationalDatabaseDefinitionCmd represents the aws-relational-database-definition command
var CreateAwsRelationalDatabaseDefinitionCmd = &cobra.Command{
	Use:          "aws-relational-database-definition",
	Example:      "tptctl create aws-relational-database-definition --config /path/to/config.yaml",
	Short:        "Create a new AWS relational database definition",
	Long:         `Create a new AWS relational database definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load AWS relational database definition config
		configContent, err := os.ReadFile(createAwsRelationalDatabaseDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsRelationalDatabaseDefinitionConfig config.AwsRelationalDatabaseDefinitionConfig
		if err := yaml.UnmarshalStrict(configContent, &awsRelationalDatabaseDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create AWS relational database definition
		awsRelationalDatabaseDefinition := awsRelationalDatabaseDefinitionConfig.AwsRelationalDatabaseDefinition
		wd, err := awsRelationalDatabaseDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create AWS relational database definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS relational database definition %s created", *wd.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateAwsRelationalDatabaseDefinitionCmd)

	CreateAwsRelationalDatabaseDefinitionCmd.Flags().StringVarP(
		&createAwsRelationalDatabaseDefinitionConfigPath,
		"config", "c", "", "Path to file with AWS relational database definition config.",
	)
	CreateAwsRelationalDatabaseDefinitionCmd.MarkFlagRequired("config")
}
