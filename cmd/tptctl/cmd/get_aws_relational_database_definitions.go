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

// GetAwsRelationalDatabaseDefinitionsCmd represents the aws-relational-database-definitions command
var GetAwsRelationalDatabaseDefinitionsCmd = &cobra.Command{
	Use:          "aws-relational-database-definitions",
	Example:      "tptctl get aws-relational-database-definitions",
	Short:        "Get AWS relational database definitions from the system",
	Long:         `Get AWS relational database definitions from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, _ := getClientContext(cmd)

		// get AWS relational database definitions
		awsRelationalDatabaseDefinitions, err := client.GetAwsRelationalDatabaseDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve AWS relational database definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*awsRelationalDatabaseDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No AWS relational database definitions currently managed by %s threeport control plane",
				threeportConfig.CurrentControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t ENGINE\t ENGINE VERSION\t DATABASE NAME\t DATABASE_PORT\t BACKUP DAYS\t MACHINE SIZE\t STORAGE GB\t WORKLOAD SECRET NAME\t AWS ACCOUNT NAME\t AGE")
		metadataErr := false
		var awsAccountErr error
		for _, r := range *awsRelationalDatabaseDefinitions {
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
				writer, *r.Name, "\t", *r.Engine, "\t", *r.EngineVersion, "\t",
				*r.DatabaseName, "\t", *r.DatabasePort, "\t", *r.BackupDays, "\t",
				*r.MachineSize, "\t", *r.StorageGb, "\t", *r.WorkloadSecretName, "\t",
				awsAccountName, "\t", util.GetAge(r.CreatedAt),
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
	GetCmd.AddCommand(GetAwsRelationalDatabaseDefinitionsCmd)
}
