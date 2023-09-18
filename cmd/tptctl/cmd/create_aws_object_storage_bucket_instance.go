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

var createAwsObjectStorageBucketInstanceConfigPath string

// CreateAwsObjectStorageBucketInstanceCmd represents the aws-object-storage-bucket-instance command
var CreateAwsObjectStorageBucketInstanceCmd = &cobra.Command{
	Use:          "aws-object-storage-bucket-instance",
	Example:      "tptctl create aws-object-storage-bucket-instance --config /path/to/config.yaml",
	Short:        "Create a new AWS object storage bucket instance",
	Long:         `Create a new AWS object storage bucket instance.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// load AWS object storage bucket instance config
		configContent, err := ioutil.ReadFile(createAwsObjectStorageBucketInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsObjectStorageBucketInstanceConfig config.AwsObjectStorageBucketInstanceConfig
		if err := yaml.Unmarshal(configContent, &awsObjectStorageBucketInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled()
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
		}

		// create AWS object storage bucket instance
		awsObjectStorageBucketInstance := awsObjectStorageBucketInstanceConfig.AwsObjectStorageBucketInstance
		kri, err := awsObjectStorageBucketInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create AWS object storage bucket instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS object storage bucket instance %s created\n", *kri.Name))
	},
}

func init() {
	createCmd.AddCommand(CreateAwsObjectStorageBucketInstanceCmd)

	CreateAwsObjectStorageBucketInstanceCmd.Flags().StringVarP(
		&createAwsObjectStorageBucketInstanceConfigPath,
		"config", "c", "", "Path to file with AWS object storage bucket instance config.",
	)
	CreateAwsObjectStorageBucketInstanceCmd.MarkFlagRequired("config")
}
