/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GetAwsObjectStorageBucketDefinitionsCmd represents the aws-object-storage-bucket-definitions command
var GetAwsObjectStorageBucketDefinitionsCmd = &cobra.Command{
	Use:          "aws-object-storage-bucket-definitions",
	Example:      "tptctl get aws-object-storage-bucket-definitions",
	Short:        "Get AWS object storage bucket definitions from the system",
	Long:         `Get AWS object storage bucket definitions from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, _ := getClientContext(cmd)

		// get AWS object storage bucket definitions
		awsObjectStorageBucketDefinitions, err := client.GetAwsObjectStorageBucketDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve AWS object storage bucket definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*awsObjectStorageBucketDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No AWS object storage bucket definitions currently managed by %s threeport control plane",
				threeportConfig.CurrentControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t PUBLIC READ ACCESS\t WORKLOAD SERVICE ACCOUNT NAME\t WORKLOAD BUCKET CONFIG MAP\t AWS ACCOUNT NAME\t AGE")
		metadataErr := false
		var awsAccountErr error
		for _, r := range *awsObjectStorageBucketDefinitions {
			var awsAccountName string
			awsAccount, err := client.GetAwsAccountByID(apiClient, apiEndpoint, *r.AwsAccountID)
			if err != nil {
				awsAccountName = "<error>"
				metadataErr = true
				awsAccountErr = err
			} else {
				awsAccountName = *awsAccount.Name
			}
			fmt.Fprintln(
				writer, *r.Name, "\t", *r.PublicReadAccess, "\t", *r.WorkloadServiceAccountName, "\t",
				*r.WorkloadBucketEnvVar, "\t", awsAccountName, "\t", util.GetAge(r.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if awsAccountErr != nil {
				cli.Error("encountered an error retrieving AWS account info", awsAccountErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	GetCmd.AddCommand(GetAwsObjectStorageBucketDefinitionsCmd)
}
