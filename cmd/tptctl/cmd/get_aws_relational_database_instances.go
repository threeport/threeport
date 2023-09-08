/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/util"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// GetAwsRelationalDatabaseInstancesCmd represents the aws-relational-database-instances command
var GetAwsRelationalDatabaseInstancesCmd = &cobra.Command{
	Use:          "aws-relational-database-instances",
	Example:      "tptctl get aws-relational-database-instances",
	Short:        "Get AWS relational database instances from the system",
	Long:         `Get AWS relational database instances from the system.`,
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
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
		}

		// get AWS relational database instances
		awsRelationalDatabaseInstances, err := client.GetAwsRelationalDatabaseInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve AWS relational database instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*awsRelationalDatabaseInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No AWS relational database instances currently managed by %s threeport control plane",
				threeportConfig.CurrentInstance,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t AWS RELATIONAL DATABASE DEFINITION\t WORKLOAD INSTANCE\t AGE")
		metadataErr := false
		var awsRelationalDatabaseDefErr error
		var workloadInstErr error
		for _, r := range *awsRelationalDatabaseInstances {
			var awsRelationalDatabaseDefName string
			awsRelationalDatabaseDefinition, err := client.GetAwsRelationalDatabaseDefinitionByID(
				apiClient,
				apiEndpoint,
				*r.AwsRelationalDatabaseDefinitionID,
			)
			if err != nil {
				metadataErr = true
				awsRelationalDatabaseDefErr = err
				awsRelationalDatabaseDefName = "<error>"
			} else {
				awsRelationalDatabaseDefName = *awsRelationalDatabaseDefinition.Name
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
				writer, *r.Name, "\t", awsRelationalDatabaseDefName, "\t", workloadInstName, "\t",
				util.GetAge(r.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if awsRelationalDatabaseDefErr != nil {
				cli.Error("encountered an error retrieving AWS relational database definition info", awsRelationalDatabaseDefErr)
			}
			if workloadInstErr != nil {
				cli.Error("encountered an error retrieving workload instance info", workloadInstErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	getCmd.AddCommand(GetAwsRelationalDatabaseInstancesCmd)
}
