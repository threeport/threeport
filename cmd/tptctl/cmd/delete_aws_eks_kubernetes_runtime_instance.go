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

var deleteAwsEksKubernetesRuntimeInstanceName string

// DeleteAwsEksKubernetesRuntimeInstanceCmd represents the aws-eks-kubernetes-runtime-instance command
var DeleteAwsEksKubernetesRuntimeInstanceCmd = &cobra.Command{
	Use:          "aws-eks-kubernetes-runtime-instance",
	Example:      "tptctl delete aws-eks-kubernetes-runtime-instance --name some-instance",
	Short:        "Delete an existing AWS EKS kubernetes runtime instance",
	Long:         `Delete an existing AWS EKS kubernetes runtime instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		awsEksKubernetesRuntimeInstanceConfig := config.AwsEksKubernetesRuntimeInstanceConfig{
			AwsEksKubernetesRuntimeInstance: config.AwsEksKubernetesRuntimeInstanceValues{
				Name: deleteAwsEksKubernetesRuntimeInstanceName,
			},
		}

		// delete AWS EKS kubernetes runtime instance
		awsEksKubernetesRuntimeInstance := awsEksKubernetesRuntimeInstanceConfig.AwsEksKubernetesRuntimeInstance
		aa, err := awsEksKubernetesRuntimeInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete AWS EKS kubernetes runtime instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("AWS EKS kubernetes runtime instance %s deleted", *aa.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteAwsEksKubernetesRuntimeInstanceCmd)

	DeleteAwsEksKubernetesRuntimeInstanceCmd.Flags().StringVarP(
		&deleteAwsEksKubernetesRuntimeInstanceName,
		"name", "n", "", "Name of AWS EKS kubernetes runtime instance.",
	)
	DeleteAwsEksKubernetesRuntimeInstanceCmd.MarkFlagRequired("name")
	DeleteAwsEksKubernetesRuntimeInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
