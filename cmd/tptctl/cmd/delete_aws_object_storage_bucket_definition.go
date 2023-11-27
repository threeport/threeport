/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
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
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		awsObjectStorageBucketDefinitionConfig := config.AwsObjectStorageBucketDefinitionConfig{
			AwsObjectStorageBucketDefinition: config.AwsObjectStorageBucketDefinitionValues{
				Name: deleteAwsObjectStorageBucketDefinitionName,
			},
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
	DeleteCmd.AddCommand(DeleteAwsObjectStorageBucketDefinitionCmd)

	DeleteAwsObjectStorageBucketDefinitionCmd.Flags().StringVarP(
		&deleteAwsObjectStorageBucketDefinitionName,
		"name", "n", "", "Name of AWS object storage bucket definition.",
	)
	DeleteAwsObjectStorageBucketDefinitionCmd.MarkFlagRequired("name")
}
