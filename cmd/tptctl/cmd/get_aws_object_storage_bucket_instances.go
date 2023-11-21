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

// GetAwsObjectStorageBucketInstancesCmd represents the aws-object-storage-bucket-instances command
var GetAwsObjectStorageBucketInstancesCmd = &cobra.Command{
	Use:          "aws-object-storage-bucket-instances",
	Example:      "tptctl get aws-object-storage-bucket-instances",
	Short:        "Get AWS object storage bucket instances from the system",
	Long:         `Get AWS object storage bucket instances from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, _ := getClientContext(cmd)

		// get AWS object storage bucket instances
		awsObjectStorageBucketInstances, err := client.GetAwsObjectStorageBucketInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve AWS object storage bucket instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*awsObjectStorageBucketInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No AWS object storage bucket instances currently managed by %s threeport control plane",
				threeportConfig.CurrentControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t AWS OBJECT STORAGE BUCKET DEFINITION\t WORKLOAD INSTANCE\t AGE")
		metadataErr := false
		var awsObjectStorageBucketDefErr error
		var workloadInstErr error
		for _, r := range *awsObjectStorageBucketInstances {
			var awsObjectStorageBucketDefName string
			awsObjectStorageBucketDefinition, err := client.GetAwsObjectStorageBucketDefinitionByID(
				apiClient,
				apiEndpoint,
				*r.AwsObjectStorageBucketDefinitionID,
			)
			if err != nil {
				metadataErr = true
				awsObjectStorageBucketDefErr = err
				awsObjectStorageBucketDefName = "<error>"
			} else {
				awsObjectStorageBucketDefName = *awsObjectStorageBucketDefinition.Name
			}
			var workloadInstName string
			workloadInstance, err := client.GetWorkloadInstanceByID(
				apiClient,
				apiEndpoint,
				*r.WorkloadInstanceID,
			)
			if err != nil {
				metadataErr = true
				workloadInstErr = err
				workloadInstName = "<error>"
			} else {
				workloadInstName = *workloadInstance.Name
			}
			fmt.Fprintln(
				writer, *r.Name, "\t", awsObjectStorageBucketDefName, "\t", workloadInstName, "\t",
				util.GetAge(r.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if awsObjectStorageBucketDefErr != nil {
				cli.Error("encountered an error retrieving AWS object storage bucket definition info", awsObjectStorageBucketDefErr)
			}
			if workloadInstErr != nil {
				cli.Error("encountered an error retrieving workload instance info", workloadInstErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	GetCmd.AddCommand(GetAwsObjectStorageBucketInstancesCmd)
}
