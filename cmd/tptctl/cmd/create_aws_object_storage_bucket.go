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

var createAwsObjectStorageBucketConfigPath string

// CreateAwsObjectStorageBucketCmd represents the aws-object-storage-bucket command
var CreateAwsObjectStorageBucketCmd = &cobra.Command{
	Use:     "aws-object-storage-bucket",
	Example: "tptctl create aws-object-storage-bucket --config /path/to/config.yaml",
	Short:   "Create a new AWS object storage bucket",
	Long: `Create a new AWS object storage bucket. This command creates a new AWS object storage bucket definition
and AWS object storage bucket instance based on the AWS object storage bucket config.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load AWS object storage bucket config
		configContent, err := os.ReadFile(createAwsObjectStorageBucketConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsObjectStorageBucketConfig config.AwsObjectStorageBucketConfig
		if err := yaml.UnmarshalStrict(configContent, &awsObjectStorageBucketConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create AWS object storage bucket
		awsObjectStorageBucket := awsObjectStorageBucketConfig.AwsObjectStorageBucket
		rd, ri, err := awsObjectStorageBucket.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create AWS object storage bucket", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("AWS object storage bucket definition %s created", *rd.Name))
		cli.Info(fmt.Sprintf("AWS object storage bucket instance %s created", *ri.Name))
		cli.Complete(fmt.Sprintf("AWS object storage bucket %s created", awsObjectStorageBucketConfig.AwsObjectStorageBucket.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateAwsObjectStorageBucketCmd)

	CreateAwsObjectStorageBucketCmd.Flags().StringVarP(
		&createAwsObjectStorageBucketConfigPath,
		"config", "c", "", "Path to file with AWS object storage bucket config.",
	)
	CreateAwsObjectStorageBucketCmd.MarkFlagRequired("config")
}
