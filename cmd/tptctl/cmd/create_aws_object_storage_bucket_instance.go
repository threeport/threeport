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

var createAwsObjectStorageBucketInstanceConfigPath string

// CreateAwsObjectStorageBucketInstanceCmd represents the aws-object-storage-bucket-instance command
var CreateAwsObjectStorageBucketInstanceCmd = &cobra.Command{
	Use:          "aws-object-storage-bucket-instance",
	Example:      "tptctl create aws-object-storage-bucket-instance --config /path/to/config.yaml",
	Short:        "Create a new AWS object storage bucket instance",
	Long:         `Create a new AWS object storage bucket instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load AWS object storage bucket instance config
		configContent, err := os.ReadFile(createAwsObjectStorageBucketInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsObjectStorageBucketInstanceConfig config.AwsObjectStorageBucketInstanceConfig
		if err := yaml.Unmarshal(configContent, &awsObjectStorageBucketInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
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
	CreateCmd.AddCommand(CreateAwsObjectStorageBucketInstanceCmd)

	CreateAwsObjectStorageBucketInstanceCmd.Flags().StringVarP(
		&createAwsObjectStorageBucketInstanceConfigPath,
		"config", "c", "", "Path to file with AWS object storage bucket instance config.",
	)
	CreateAwsObjectStorageBucketInstanceCmd.MarkFlagRequired("config")
}
