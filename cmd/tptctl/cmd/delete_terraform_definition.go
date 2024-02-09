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

var (
	deleteTerraformDefinitionConfigPath string
	deleteTerraformDefinitionName       string
)

// DeleteTerraformDefinitionCmd represents the terraform-definition command
var DeleteTerraformDefinitionCmd = &cobra.Command{
	Use:          "terraform-definition",
	Example:      "tptctl delete terraform-definition --config /path/to/config.yaml",
	Short:        "Delete an existing terraform definition",
	Long:         `Delete an existing terraform definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {

		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteTerraformDefinitionConfigPath,
			deleteTerraformDefinitionName,
			"terraform definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var terraformDefinitionConfig config.TerraformDefinitionConfig
		if deleteTerraformDefinitionConfigPath != "" {
			// load terraform definition config
			configContent, err := os.ReadFile(deleteTerraformDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &terraformDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			terraformDefinitionConfig = config.TerraformDefinitionConfig{
				TerraformDefinition: config.TerraformDefinitionValues{
					Name: deleteTerraformDefinitionName,
				},
			}
		}

		// delete terraform definition
		terraformDefinition := terraformDefinitionConfig.TerraformDefinition
		_, err := terraformDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete terraform definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("terraform definition %s deleted", terraformDefinition.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteTerraformDefinitionCmd)

	DeleteTerraformDefinitionCmd.Flags().StringVarP(
		&deleteTerraformDefinitionConfigPath,
		"config", "c", "", "Path to file with terraform definition config.",
	)
	DeleteTerraformDefinitionCmd.Flags().StringVarP(
		&deleteTerraformDefinitionName,
		"name", "n", "", "Name of terraform definition.",
	)
	DeleteTerraformDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
