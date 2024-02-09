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

var createKubernetesRuntimeDefinitionConfigPath string

// CreateKubernetesRuntimeDefinitionCmd represents the kubernetes-runtime-definition command
var CreateKubernetesRuntimeDefinitionCmd = &cobra.Command{
	Use:          "kubernetes-runtime-definition",
	Example:      "tptctl create kubernetes-runtime-definition --config /path/to/config.yaml",
	Short:        "Create a new kubernetes runtime definition",
	Long:         `Create a new kubernetes runtime definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load kubernetes runtime definition config
		configContent, err := os.ReadFile(createKubernetesRuntimeDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var kubernetesRuntimeDefinitionConfig config.KubernetesRuntimeDefinitionConfig
		if err := yaml.UnmarshalStrict(configContent, &kubernetesRuntimeDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create kubernetes runtime definition
		kubernetesRuntimeDefinition := kubernetesRuntimeDefinitionConfig.KubernetesRuntimeDefinition
		wd, err := kubernetesRuntimeDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create kubernetes runtime definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("kubernetes runtime definition %s created", *wd.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateKubernetesRuntimeDefinitionCmd)

	CreateKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&createKubernetesRuntimeDefinitionConfigPath,
		"config", "c", "", "Path to file with kubernetes runtime definition config.",
	)
	CreateKubernetesRuntimeDefinitionCmd.MarkFlagRequired("config")
	CreateKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
