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
	deleteTerraformInstanceConfigPath string
	deleteTerraformInstanceName       string
)

// DeleteTerraformInstanceCmd represents the terraform-instance command
var DeleteTerraformInstanceCmd = &cobra.Command{
	Use:          "terraform-instance",
	Example:      "tptctl delete terraform-instance --config /path/to/config.yaml",
	Short:        "Delete an existing terraform instance",
	Long:         `Delete an existing terraform instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteTerraformInstanceConfigPath,
			deleteTerraformInstanceName,
			"terraform instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var terraformInstanceConfig config.TerraformInstanceConfig
		if deleteTerraformInstanceConfigPath != "" {
			// load terraform instance config
			configContent, err := os.ReadFile(deleteTerraformInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &terraformInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			terraformInstanceConfig = config.TerraformInstanceConfig{
				TerraformInstance: config.TerraformInstanceValues{
					Name: deleteTerraformInstanceName,
				},
			}
		}

		// delete terraform instance
		terraformInstance := terraformInstanceConfig.TerraformInstance
		_, err := terraformInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete terraform instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("terraform instance %s deleted\n", terraformInstance.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteTerraformInstanceCmd)

	DeleteTerraformInstanceCmd.Flags().StringVarP(
		&deleteTerraformInstanceConfigPath,
		"config", "c", "", "Path to file with terraform instance config.",
	)
	DeleteTerraformInstanceCmd.Flags().StringVarP(
		&deleteTerraformInstanceName,
		"name", "n", "", "Name of terraform instance.",
	)
	DeleteTerraformInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
