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

var deleteAwsObjectStorageBucketDefinitionName string

// DeleteAwsObjectStorageBucketDefinitionCmd represents the aws-object-storage-bucket-definition command
var DeleteAwsObjectStorageBucketDefinitionCmd = &cobra.Command{
	Use:          "aws-object-storage-bucket-definition",
	Example:      "tptctl delete aws-object-storage-bucket-definition --name some-definition",
	Short:        "Delete an existing AWS object storage bucket definition",
	Long:         `Delete an existing AWS object storage bucket definition.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {

		// get threeport config and extract threeport API endpoint
		threeportConfig, err := config.GetThreeportConfig(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		awsObjectStorageBucketDefinitionConfig := config.AwsObjectStorageBucketDefinitionConfig{
			AwsObjectStorageBucketDefinition: config.AwsObjectStorageBucketDefinitionValues{
				Name: deleteAwsObjectStorageBucketDefinitionName,
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

		// delete AWS object storage bucket definition
		awsObjectStorageBucketDefinition := awsObjectStorageBucketDefinitionConfig.AwsObjectStorageBucketDefinition
		aa, err := awsObjectStorageBucketDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS object storage bucket definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS object storage bucket definition %s deleted", *aa.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteAwsObjectStorageBucketDefinitionCmd)

	DeleteAwsObjectStorageBucketDefinitionCmd.Flags().StringVarP(
		&deleteAwsObjectStorageBucketDefinitionName,
		"name", "n", "", "Name of AWS object storage bucket definition.",
	)
	DeleteAwsObjectStorageBucketDefinitionCmd.MarkFlagRequired("name")
}
