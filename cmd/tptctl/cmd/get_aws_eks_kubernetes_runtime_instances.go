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
	config "github.com/threeport/threeport/pkg/config/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GetAwsEksKubernetesRuntimeDefinitionsCmd represents the aws-eks-kubernetes-runtime-definitions command
var GetAwsEksKubernetesRuntimeDefinitionsCmd = &cobra.Command{
	Use:          "aws-eks-kubernetes-runtime-definitions",
	Example:      "tptctl get aws-eks-kubernetes-runtime-definitions",
	Short:        "Get AWS EKS kubernetes runtime definitions from the system",
	Long:         `Get AWS EKS kubernetes runtime definitions from the system.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedInstance, err := config.GetThreeportConfig(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedInstance)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForInstance(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
		}

		// get AWS EKS kubernetes runtime definitions
		awsEksKubernetesRuntimeDefinitions, err := client.GetAwsEksKubernetesRuntimeDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve AWS EKS kubernetes runtime definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*awsEksKubernetesRuntimeDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No AWS EKS kubernetes runtime definitions currently managed by %s threeport control plane",
				requestedInstance,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t AWS ACCOUNT\t ZONE COUNT\t DEFAULT NODE GROUP INSTANCE TYPE\t DEFAULT NODE GROUP MINIMUM SIZE\t DEFAULT NODE GROUP MAXIMUM SIZE\t AGE")
		metadataErr := false
		var awsAccountErr error
		for _, aekrd := range *awsEksKubernetesRuntimeDefinitions {
			// get AWS account name
			var awsAccountName string
			awsAccount, err := client.GetAwsAccountByID(apiClient, apiEndpoint, *aekrd.AwsAccountID)
			if err != nil {
				metadataErr = true
				awsAccountErr = err
				awsAccountName = "<error>"
			}
			awsAccountName = *awsAccount.Name
			fmt.Fprintln(
				writer, *aekrd.Name, "\t", awsAccountName, "\t", *aekrd.ZoneCount, "\t", *aekrd.DefaultNodeGroupInstanceType, "\t",
				*aekrd.DefaultNodeGroupMinimumSize, "\t", *aekrd.DefaultNodeGroupMaximumSize, "\t", util.GetAge(aekrd.CreatedAt),
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
	getCmd.AddCommand(GetAwsEksKubernetesRuntimeDefinitionsCmd)
}
