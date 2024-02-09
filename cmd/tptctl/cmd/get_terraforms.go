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

// GetTerraformsCmd represents the terraforms command
var GetTerraformsCmd = &cobra.Command{
	Use:     "terraforms",
	Example: "tptctl get terraforms",
	Short:   "Get terraform resource deployments from the system",
	Long: `Get terraform resource deployments from the system.

A terraform is a simple abstraction of terraform definitions and terraform instances.
This command displays all instances and the definitions used to configure them.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)
		// get terraform instances
		terraformInstances, err := client.GetTerraformInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve terraform instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*terraformInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No terraforms currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t TERRAFORM DEFINITION\t TERRAFORM INSTANCE\t AWS ACCOUNT\t STATUS\t AGE")
		metadataErr := false
		var terraformDefErr error
		var awsAccountErr error
		var statusErr error
		for _, terraformInst := range *terraformInstances {
			// get terraform definition name for instance
			var terraformDef string
			terraformDefinition, err := client.GetTerraformDefinitionByID(apiClient, apiEndpoint, *terraformInst.TerraformDefinitionID)
			if err != nil {
				metadataErr = true
				terraformDefErr = err
				terraformDef = "<error>"
			} else {
				terraformDef = *terraformDefinition.Name
			}
			// get AWS acocunt name
			var awsAccountName string
			awsAccount, err := client.GetAwsAccountByID(apiClient, apiEndpoint, *terraformInst.AwsAccountID)
			if err != nil {
				metadataErr = true
				awsAccountErr = err
				awsAccountName = "<error>"
			} else {
				awsAccountName = *awsAccount.Name
			}
			// get terraform instance status
			terraformInstStatus := "Reconciling"
			if *terraformInst.Reconciled {
				terraformInstStatus = "Healthy"
			}
			if *terraformInst.CreationFailed {
				terraformInstStatus = "Failed"
			}
			fmt.Fprintln(
				writer, terraformDef, "\t", terraformDef, "\t", *terraformInst.Name, "\t", awsAccountName, "\t",
				terraformInstStatus, "\t", util.GetAge(terraformInst.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if terraformDefErr != nil {
				cli.Error("encountered an error retrieving terraform definition info", terraformDefErr)
			}
			if awsAccountErr != nil {
				cli.Error("encountered an error retrieving AWS account info", terraformDefErr)
			}
			if statusErr != nil {
				cli.Error("encountered an error retrieving terraform instance status", statusErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	GetCmd.AddCommand(GetTerraformsCmd)
	GetTerraformsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
