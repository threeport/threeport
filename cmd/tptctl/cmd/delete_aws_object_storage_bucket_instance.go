/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var deleteAwsObjectStorageBucketInstanceName string

// DeleteAwsObjectStorageBucketInstanceCmd represents the aws-object-storage-bucket-instance command
var DeleteAwsObjectStorageBucketInstanceCmd = &cobra.Command{
	Use:          "aws-object-storage-bucket-instance",
	Example:      "tptctl delete aws-object-storage-bucket-instance --name some-instance",
	Short:        "Delete an existing AWS object storage bucket instance",
	Long:         `Delete an existing AWS object storage bucket instance.`,
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

		awsObjectStorageBucketInstanceConfig := config.AwsObjectStorageBucketInstanceConfig{
			AwsObjectStorageBucketInstance: config.AwsObjectStorageBucketInstanceValues{
				Name: deleteAwsObjectStorageBucketInstanceName,
			},
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
			cli.Error("failed to create https client", err)
			os.Exit(1)
		}

		// delete AWS object storage bucket instance
		awsObjectStorageBucketInstance := awsObjectStorageBucketInstanceConfig.AwsObjectStorageBucketInstance
		aa, err := awsObjectStorageBucketInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS object storage bucket instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS object storage bucket instance %s deleted", *aa.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteAwsObjectStorageBucketInstanceCmd)

	DeleteAwsObjectStorageBucketInstanceCmd.Flags().StringVarP(
		&deleteAwsObjectStorageBucketInstanceName,
		"name", "n", "", "Name of AWS object storage bucket instance.",
	)
	DeleteAwsObjectStorageBucketInstanceCmd.MarkFlagRequired("name")
}
