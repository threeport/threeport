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

var createAwsRelationalDatabaseDefinitionConfigPath string

// CreateAwsRelationalDatabaseDefinitionCmd represents the aws-relational-database-definition command
var CreateAwsRelationalDatabaseDefinitionCmd = &cobra.Command{
	Use:          "aws-relational-database-definition",
	Example:      "tptctl create aws-relational-database-definition --config /path/to/config.yaml",
	Short:        "Create a new AWS relational database definition",
	Long:         `Create a new AWS relational database definition.`,
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

		// load AWS relational database definition config
		configContent, err := ioutil.ReadFile(createAwsRelationalDatabaseDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsRelationalDatabaseDefinitionConfig config.AwsRelationalDatabaseDefinitionConfig
		if err := yaml.Unmarshal(configContent, &awsRelationalDatabaseDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create threeport API client", err)
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
