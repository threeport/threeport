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

var deleteAwsEksKubernetesRuntimeDefinitionName string

// DeleteAwsEksKubernetesRuntimeDefinitionCmd represents the aws-eks-kubernetes-runtime-definition command
var DeleteAwsEksKubernetesRuntimeDefinitionCmd = &cobra.Command{
	Use:          "aws-eks-kubernetes-runtime-definition",
	Example:      "tptctl delete aws-eks-kubernetes-runtime-definition --name some-definition",
	Short:        "Delete an existing AWS EKS kubernetes runtime definition",
	Long:         `Delete an existing AWS EKS kubernetes runtime definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

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
