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

var createHelmWorkloadDefinitionConfigPath string

// CreateHelmWorkloadDefinitionCmd represents the helm-workload-definition command
var CreateHelmWorkloadDefinitionCmd = &cobra.Command{
	Use:          "helm-workload-definition",
	Example:      "tptctl create helm-workload-definition --config /path/to/config.yaml",
	Short:        "Create a new helm workload definition",
	Long:         `Create a new helm workload definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load helm workload definition config
		configContent, err := os.ReadFile(createHelmWorkloadDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var helmWorkloadDefinitionConfig config.HelmWorkloadDefinitionConfig
		if err := yaml.UnmarshalStrict(configContent, &helmWorkloadDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create helm workload definition
		helmWorkloadDefinition := helmWorkloadDefinitionConfig.HelmWorkloadDefinition
		helmWorkloadDefinition.HelmWorkloadConfigPath = createHelmWorkloadDefinitionConfigPath
		helmWorkloadDef, err := helmWorkloadDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create helm workload definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("helm workload definition %s created", *helmWorkloadDef.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateHelmWorkloadDefinitionCmd)

	CreateHelmWorkloadDefinitionCmd.Flags().StringVarP(
		&createHelmWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with helm workload definition config.",
	)
	CreateHelmWorkloadDefinitionCmd.MarkFlagRequired("config")
	CreateHelmWorkloadDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
