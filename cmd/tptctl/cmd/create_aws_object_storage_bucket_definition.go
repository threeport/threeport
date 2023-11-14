/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createAwsObjectStorageBucketDefinitionConfigPath string

// CreateAwsObjectStorageBucketDefinitionCmd represents the aws-object-storage-bucket-definition command
var CreateAwsObjectStorageBucketDefinitionCmd = &cobra.Command{
	Use:          "aws-object-storage-bucket-definition",
	Example:      "tptctl create aws-object-storage-bucket-definition --config /path/to/config.yaml",
	Short:        "Create a new AWS object storage bucket definition",
	Long:         `Create a new AWS object storage bucket definition.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		var apiClient *http.Client
		var apiEndpoint string

		apiClient, apiEndpoint = checkContext(cmd)
		if apiClient == nil && apiEndpoint != "" {
			apiEndpoint, err = threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
			apiClient, err = threeportConfig.GetHTTPClient(requestedControlPlane)
			if err != nil {
				cli.Error("failed to create threeport API client", err)
				os.Exit(1)
			}
		}

		// load AWS object storage bucket definition config
		configContent, err := os.ReadFile(createAwsObjectStorageBucketDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsObjectStorageBucketDefinitionConfig config.AwsObjectStorageBucketDefinitionConfig
		if err := yaml.Unmarshal(configContent, &awsObjectStorageBucketDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create AWS object storage bucket definition
		awsObjectStorageBucketDefinition := awsObjectStorageBucketDefinitionConfig.AwsObjectStorageBucketDefinition
		wd, err := awsObjectStorageBucketDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create AWS object storage bucket definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS object storage bucket definition %s created", *wd.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateAwsObjectStorageBucketDefinitionCmd)

	CreateAwsObjectStorageBucketDefinitionCmd.Flags().StringVarP(
		&createAwsObjectStorageBucketDefinitionConfigPath,
		"config", "c", "", "Path to file with AWS object storage bucket definition config.",
	)
	CreateAwsObjectStorageBucketDefinitionCmd.MarkFlagRequired("config")
}
