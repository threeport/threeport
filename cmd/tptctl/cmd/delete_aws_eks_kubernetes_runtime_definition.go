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

		awsEksKubernetesRuntimeDefinitionConfig := config.AwsEksKubernetesRuntimeDefinitionConfig{
			AwsEksKubernetesRuntimeDefinition: config.AwsEksKubernetesRuntimeDefinitionValues{
				Name: deleteAwsEksKubernetesRuntimeDefinitionName,
			},
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
	DeleteCmd.AddCommand(DeleteAwsEksKubernetesRuntimeDefinitionCmd)

	DeleteAwsEksKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&deleteAwsEksKubernetesRuntimeDefinitionName,
		"name", "n", "", "Name of AWS EKS kubernetes runtime definition.",
	)
	DeleteAwsEksKubernetesRuntimeDefinitionCmd.MarkFlagRequired("name")
	DeleteAwsEksKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
