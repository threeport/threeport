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
	deleteAwsObjectStorageBucketConfigPath string
	deleteAwsObjectStorageBucketName       string
)

// DeleteAwsObjectStorageBucketCmd represents the aws-object-storage-bucket command
var DeleteAwsObjectStorageBucketCmd = &cobra.Command{
	Use:     "aws-object-storage-bucket",
	Example: "tptctl delete aws-object-storage-bucket --config /path/to/config.yaml",
	Short:   "Delete an existing AWS object storage bucket",
	Long: `Delete an existing AWS object storage bucket. This command deletes an existing
AWS object storage bucket definition and AWS object storage bucket instance based on
the AWS object storage bucket config or name.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteAwsObjectStorageBucketConfigPath,
			deleteAwsObjectStorageBucketName,
			"AWS object storage bucket",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var awsObjectStorageBucketConfig config.AwsObjectStorageBucketConfig
		if deleteAwsObjectStorageBucketConfigPath != "" {
			// load AWS object storage bucket definition config
			configContent, err := os.ReadFile(deleteAwsObjectStorageBucketConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &awsObjectStorageBucketConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			awsObjectStorageBucketConfig = config.AwsObjectStorageBucketConfig{
				AwsObjectStorageBucket: config.AwsObjectStorageBucketValues{
					Name: deleteAwsObjectStorageBucketName,
				},
			}
		}

		// delete AWS object storage bucket
		cli.Info("deleting AWS object storage bucket (this will take a few minutes)...")
		awsObjectStorageBucket := awsObjectStorageBucketConfig.AwsObjectStorageBucket
		rd, ri, err := awsObjectStorageBucket.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS object storage bucket", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("AWS object storage bucket instance %s deleted", *ri.Name))
		cli.Info(fmt.Sprintf("AWS object storage bucket definition %s deleted", *rd.Name))
		cli.Complete(fmt.Sprintf("AWS object storage bucket %s deleted", awsObjectStorageBucketConfig.AwsObjectStorageBucket.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteAwsObjectStorageBucketCmd)

	DeleteAwsObjectStorageBucketCmd.Flags().StringVarP(
		&deleteAwsObjectStorageBucketConfigPath,
		"config", "c", "", "Path to file with AWS object storage bucket config.",
	)
	DeleteAwsObjectStorageBucketCmd.Flags().StringVarP(
		&deleteAwsObjectStorageBucketName,
		"name", "n", "", "Name of AWS object storage bucket.",
	)
}
