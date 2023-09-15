/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedInstance, err := config.GetThreeportConfig(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// load AWS relational database config
		configContent, err := ioutil.ReadFile(createAwsRelationalDatabaseConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsRelationalDatabaseConfig config.AwsRelationalDatabaseConfig
		if err := yaml.Unmarshal(configContent, &awsRelationalDatabaseConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedInstance)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForInstance(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create https client", err)
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
	createCmd.AddCommand(CreateAwsRelationalDatabaseCmd)

	CreateAwsRelationalDatabaseCmd.Flags().StringVarP(
		&createAwsRelationalDatabaseConfigPath,
		"config", "c", "", "Path to file with AWS relational database config.",
	)
	CreateAwsRelationalDatabaseCmd.MarkFlagRequired("config")
}
