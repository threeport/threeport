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

var createTerraformDefinitionConfigPath string

// CreateTerraformDefinitionCmd represents the terraform-definition command
var CreateTerraformDefinitionCmd = &cobra.Command{
	Use:          "terraform-definition",
	Example:      "tptctl create terraform-definition --config /path/to/config.yaml",
	Short:        "Create a new terraform definition",
	Long:         `Create a new terraform definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load terraform definition config
		configContent, err := os.ReadFile(createTerraformDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var terraformDefinitionConfig config.TerraformDefinitionConfig
		if err := yaml.UnmarshalStrict(configContent, &terraformDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create terraform definition
		terraformDefinition := terraformDefinitionConfig.TerraformDefinition
		terraformDefinition.TerraformConfigPath = createTerraformDefinitionConfigPath
		terraformDef, err := terraformDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create terraform definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("terraform definition %s created", *terraformDef.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateTerraformDefinitionCmd)

	CreateTerraformDefinitionCmd.Flags().StringVarP(
		&createTerraformDefinitionConfigPath,
		"config", "c", "", "Path to file with terraform definition config.",
	)
	CreateTerraformDefinitionCmd.MarkFlagRequired("config")
	CreateTerraformDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
