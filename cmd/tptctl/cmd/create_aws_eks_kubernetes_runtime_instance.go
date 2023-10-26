/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createAwsEksKubernetesRuntimeInstanceConfigPath string

// CreateAwsEksKubernetesRuntimeInstanceCmd represents the aws-eks-kubernetes-runtime-instance command
var CreateAwsEksKubernetesRuntimeInstanceCmd = &cobra.Command{
	Use:          "aws-eks-kubernetes-runtime-instance",
	Example:      "tptctl create aws-eks-kubernetes-runtime-instance --config /path/to/config.yaml",
	Short:        "Create a new AWS EKS kubernetes runtime instance",
	Long:         `Create a new AWS EKS kubernetes runtime instance.`,
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

		// load AWS EKS kubernetes runtime instance config
		configContent, err := os.ReadFile(createAwsEksKubernetesRuntimeInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsEksKubernetesRuntimeInstanceConfig config.AwsEksKubernetesRuntimeInstanceConfig
		if err := yaml.Unmarshal(configContent, &awsEksKubernetesRuntimeInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// create AWS EKS kubernetes runtime instance
		awsEksKubernetesRuntimeInstance := awsEksKubernetesRuntimeInstanceConfig.AwsEksKubernetesRuntimeInstance
		kri, err := awsEksKubernetesRuntimeInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create AWS EKS kubernetes runtime instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("kubernetes runtime instance %s created\n", *kri.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateAwsEksKubernetesRuntimeInstanceCmd)

	CreateAwsEksKubernetesRuntimeInstanceCmd.Flags().StringVarP(
		&createAwsEksKubernetesRuntimeInstanceConfigPath,
		"config", "c", "", "Path to file with AWS EKS kubernetes runtime instance config.",
	)
	CreateAwsEksKubernetesRuntimeInstanceCmd.MarkFlagRequired("config")
	CreateAwsEksKubernetesRuntimeInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
