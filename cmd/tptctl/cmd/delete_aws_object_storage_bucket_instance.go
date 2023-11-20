/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
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
			if err != nil {
				cli.Error("failed to get threeport API endpoint from config", err)
				os.Exit(1)
			}

			apiClient, err = threeportConfig.GetHTTPClient(requestedControlPlane)
			if err != nil {
				cli.Error("failed to create threeport API client", err)
				os.Exit(1)
			}
		}

		awsObjectStorageBucketInstanceConfig := config.AwsObjectStorageBucketInstanceConfig{
			AwsObjectStorageBucketInstance: config.AwsObjectStorageBucketInstanceValues{
				Name: deleteAwsObjectStorageBucketInstanceName,
			},
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
	DeleteCmd.AddCommand(DeleteAwsObjectStorageBucketInstanceCmd)

	DeleteAwsObjectStorageBucketInstanceCmd.Flags().StringVarP(
		&deleteAwsObjectStorageBucketInstanceName,
		"name", "n", "", "Name of AWS object storage bucket instance.",
	)
	DeleteAwsObjectStorageBucketInstanceCmd.MarkFlagRequired("name")
}
