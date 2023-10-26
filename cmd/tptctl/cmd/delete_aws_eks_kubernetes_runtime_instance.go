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

var deleteAwsEksKubernetesRuntimeInstanceName string

// DeleteAwsEksKubernetesRuntimeInstanceCmd represents the aws-eks-kubernetes-runtime-instance command
var DeleteAwsEksKubernetesRuntimeInstanceCmd = &cobra.Command{
	Use:          "aws-eks-kubernetes-runtime-instance",
	Example:      "tptctl delete aws-eks-kubernetes-runtime-instance --name some-instance",
	Short:        "Delete an existing AWS EKS kubernetes runtime instance",
	Long:         `Delete an existing AWS EKS kubernetes runtime instance.`,
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

			// get threeport API client
			cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
			if err != nil {
				cli.Error("failed to determine if auth is enabled on threeport API", err)
				os.Exit(1)
			}
			ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get threeport certificates from config", err)
				os.Exit(1)
			}
			apiClient, err = client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
			if err != nil {
				cli.Error("failed to create threeport API client", err)
				os.Exit(1)
			}
		}

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
