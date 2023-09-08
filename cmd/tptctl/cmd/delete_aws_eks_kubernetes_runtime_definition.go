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

var deleteAwsEksKubernetesRuntimeDefinitionName string

// DeleteAwsEksKubernetesRuntimeDefinitionCmd represents the aws-eks-kubernetes-runtime-definition command
var DeleteAwsEksKubernetesRuntimeDefinitionCmd = &cobra.Command{
	Use:          "aws-eks-kubernetes-runtime-definition",
	Example:      "tptctl delete aws-eks-kubernetes-runtime-definition --name some-definition",
	Short:        "Delete an existing AWS EKS kubernetes runtime definition",
	Long:         `Delete an existing AWS EKS kubernetes runtime definition.`,
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

		awsEksKubernetesRuntimeDefinitionConfig := config.AwsEksKubernetesRuntimeDefinitionConfig{
			AwsEksKubernetesRuntimeDefinition: config.AwsEksKubernetesRuntimeDefinitionValues{
				Name: deleteAwsEksKubernetesRuntimeDefinitionName,
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
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			cli.Error("failed to create https client", err)
			os.Exit(1)
		}

		// delete AWS EKS kubernetes runtime definition
		awsEksKubernetesRuntimeDefinition := awsEksKubernetesRuntimeDefinitionConfig.AwsEksKubernetesRuntimeDefinition
		aa, err := awsEksKubernetesRuntimeDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS EKS kubernetes runtime definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS EKS kubernetes runtime definition %s deleted", *aa.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteAwsEksKubernetesRuntimeDefinitionCmd)

	DeleteAwsEksKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&deleteAwsEksKubernetesRuntimeDefinitionName,
		"name", "n", "", "Name of AWS EKS kubernetes runtime definition.",
	)
	DeleteAwsEksKubernetesRuntimeDefinitionCmd.MarkFlagRequired("name")
}
