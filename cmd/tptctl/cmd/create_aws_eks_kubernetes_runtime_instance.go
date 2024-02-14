/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
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
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load AWS EKS kubernetes runtime instance config
		configContent, err := os.ReadFile(createAwsEksKubernetesRuntimeInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var awsEksKubernetesRuntimeInstanceConfig config.AwsEksKubernetesRuntimeInstanceConfig
		if err := yaml.UnmarshalStrict(configContent, &awsEksKubernetesRuntimeInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
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
